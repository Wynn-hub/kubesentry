package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"gorm.io/datatypes"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

func TestListEndpointsRejectInvalidPagination(t *testing.T) {
	h, _ := newDBTestServer(t)

	paths := []string{
		"/api/v1/policies?sortBy=unsafe",
		"/api/v1/policygroups?page=0",
		"/api/v1/exceptions?pageSize=25",
		"/api/v1/distributions?sortOrder=sideways",
		"/api/v1/clusters?sortBy=unsafe",
		"/api/v1/install-records?page=invalid",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec, env := doRequest(t, h, "GET", path, nil)
			if rec.Code != 400 || env.Code == 0 {
				t.Fatalf("code=%d env=%+v, want pagination validation error", rec.Code, env)
			}
		})
	}
}

func TestListEndpointsSupportPagesAndSortFields(t *testing.T) {
	h, repos := newDBTestServer(t)
	now := time.Now().UTC()
	for i := range 12 {
		name := fmt.Sprintf("zz-page-%02d", i)
		seedGroup(t, repos, "group-"+name, i%2 == 0)
		seedDistribution(t, repos, "distribution-"+name, []string{"default"})
		if err := repos.Clusters.Create(t.Context(), &dbrepo.Cluster{
			Name: name, Phase: []string{"Online", "Offline", "Registering"}[i%3], OperatorVersion: fmt.Sprintf("v%d", i),
		}); err != nil {
			t.Fatalf("seed cluster %s: %v", name, err)
		}
		if err := repos.InstallRecords.Create(t.Context(), &dbrepo.ClusterInstallRecord{
			ClusterName: name, Namespace: fmt.Sprintf("namespace-%02d", 11-i),
		}); err != nil {
			t.Fatalf("seed install record %s: %v", name, err)
		}
		expiresAt := now.Add(time.Duration(i+1) * time.Hour)
		if err := repos.Exceptions.Create(t.Context(), &dbrepo.PolicyException{
			Name: "exception-" + name,
			Spec: datatypes.NewJSONType(v1alpha1.PolicyExceptionSpec{
				AllPolicies: true,
				Duration:    fmt.Sprintf("%dh", i+1),
				Reason:      fmt.Sprintf("reason-%02d", 11-i),
			}),
			ExpiresAt: &expiresAt,
		}); err != nil {
			t.Fatalf("seed exception %s: %v", name, err)
		}
	}

	endpoints := []struct {
		path       string
		sortFields []string
	}{
		{"/api/v1/policies", []string{"name", "source", "enforcementMode", "phase", "createdAt", "updatedAt"}},
		{"/api/v1/policygroups", []string{"name", "displayName", "enabled", "phase", "resolvedCount"}},
		{"/api/v1/exceptions", []string{"name", "phase", "targetSummary", "duration", "expiresAt"}},
		{"/api/v1/distributions", []string{"name"}},
		{"/api/v1/clusters", []string{"name", "phase", "operatorVersion", "lastHeartbeatAt", "createdAt"}},
		{"/api/v1/install-records", []string{"clusterName", "namespace", "createdAt"}},
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.path, func(t *testing.T) {
			for _, sortField := range endpoint.sortFields {
				rec, env := doRequest(t, h, "GET", endpoint.path+"?pageSize=10&sortBy="+sortField+"&sortOrder=desc", nil)
				if rec.Code != 200 || env.Code != 0 {
					t.Fatalf("sortBy=%s code=%d env=%+v", sortField, rec.Code, env)
				}
				page := mustUnmarshal[testPageResult[json.RawMessage]](t, env.Data)
				if page.Page != 1 || page.PageSize != 10 || page.Total <= 10 || len(page.Items) != 10 {
					t.Fatalf("sortBy=%s page=%+v", sortField, page)
				}
			}

			rec, env := doRequest(t, h, "GET", endpoint.path+"?page=2&pageSize=10", nil)
			if rec.Code != 200 {
				t.Fatalf("second page code=%d env=%+v", rec.Code, env)
			}
			page := mustUnmarshal[testPageResult[json.RawMessage]](t, env.Data)
			if page.Page != 2 || page.PageSize != 10 || page.Total <= 10 || len(page.Items) == 0 {
				t.Fatalf("second page=%+v", page)
			}
		})
	}

	assertDerivedSortUsesFullSet(t, h, "/api/v1/policygroups", "enabled")
	assertDerivedSortUsesFullSet(t, h, "/api/v1/exceptions", "expiresAt")
}

func assertDerivedSortUsesFullSet(t *testing.T, h http.Handler, path, sortBy string) {
	t.Helper()
	_, fullEnv := doRequest(t, h, "GET", path+"?pageSize=100&sortBy="+sortBy+"&sortOrder=desc", nil)
	full := mustUnmarshal[testPageResult[json.RawMessage]](t, fullEnv.Data)
	_, secondEnv := doRequest(t, h, "GET", path+"?page=2&pageSize=10&sortBy="+sortBy+"&sortOrder=desc", nil)
	second := mustUnmarshal[testPageResult[json.RawMessage]](t, secondEnv.Data)
	if !reflect.DeepEqual(second.Items, full.Items[10:]) {
		t.Fatalf("%s sortBy=%s second page is not the sorted full-set suffix", path, sortBy)
	}
}
