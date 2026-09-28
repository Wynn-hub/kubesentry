package console

import (
	"net/http"
	"testing"
	"time"

	"gorm.io/datatypes"

	admissionobs "github.com/Wynn-hub/kubesentry/internal/admission/observability"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
	"github.com/Wynn-hub/kubesentry/internal/gatewaymodel"
)

func TestClusterScopePlatformAuthAndAdmissionScope(t *testing.T) {
	repos := repository.New(newTestDB(t))
	srv, err := NewServer(&Handlers{Repos: repos}, func() bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler
	password := "correct horse battery staple"
	if rec := authRequest(t, h, http.MethodPost, "/api/v1/setup/admin", dto.PasswordInput{Password: password}, nil, false); rec.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
	}
	adminCookie := loginCookie(t, h, "admin", password)

	createTenant := func(name string) dto.TenantView {
		rec := authRequest(t, h, http.MethodPost, "/api/v1/tenants", dto.CreateTenantRequest{Name: name}, adminCookie, true)
		return responseData[dto.TenantView](t, rec.Body.Bytes())
	}
	first, second := createTenant("cluster-first"), createTenant("cluster-second")
	createViewer := func(name string, tenantID int64) {
		rec := authRequest(t, h, http.MethodPost, "/api/v1/users", dto.CreateUserRequest{Username: name, Password: password, Role: dbrepo.RoleViewer, TenantID: &tenantID}, adminCookie, true)
		if rec.Code != http.StatusOK {
			t.Fatalf("create viewer = %d %s", rec.Code, rec.Body.String())
		}
	}
	createViewer("cluster-viewer", first.ID)

	clusterA := &dbrepo.Cluster{Name: "cluster-a", TenantID: &first.ID, Phase: "Online", Labels: datatypes.NewJSONType(map[string]string{"env": "prod"})}
	clusterB := &dbrepo.Cluster{Name: "cluster-b", TenantID: &second.ID, Phase: "Online", Labels: datatypes.NewJSONType(map[string]string{})}
	unassigned := &dbrepo.Cluster{Name: "unassigned", Phase: "Offline", Labels: datatypes.NewJSONType(map[string]string{})}
	for _, cluster := range []*dbrepo.Cluster{clusterA, clusterB, unassigned} {
		if err := repos.Clusters.Create(t.Context(), cluster); err != nil {
			t.Fatal(err)
		}
	}
	viewerCookie := loginCookie(t, h, "cluster-viewer", password)

	list := authRequest(t, h, http.MethodGet, "/api/v1/clusters", nil, viewerCookie, false)
	page := responseData[dto.ClusterPageView](t, list.Body.Bytes())
	if list.Code != http.StatusOK || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Name != "cluster-a" {
		t.Fatalf("tenant cluster list = %d %+v", list.Code, page)
	}
	if rec := authRequest(t, h, http.MethodGet, "/api/v1/clusters/cluster-b", nil, viewerCookie, false); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant cluster get = %d %s", rec.Code, rec.Body.String())
	}
	if rec := authRequest(t, h, http.MethodDelete, "/api/v1/clusters/cluster-a", nil, viewerCookie, true); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer lifecycle mutation = %d %s", rec.Code, rec.Body.String())
	}

	if err := repos.Platform.UpdateSettings(t.Context(), true, 1); err != nil {
		t.Fatal(err)
	}
	overviewRec := authRequest(t, h, http.MethodGet, "/api/v1/platform/overview", nil, viewerCookie, false)
	overview := responseData[dto.PlatformOverviewView](t, overviewRec.Body.Bytes())
	if overviewRec.Code != http.StatusOK || overview.MemberPlane.Total != 1 || len(overview.GatewayPlane) != 0 {
		t.Fatalf("tenant platform overview = %d %+v", overviewRec.Code, overview)
	}
	if rec := authRequest(t, h, http.MethodGet, "/api/v1/platform/settings", nil, viewerCookie, false); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer platform settings = %d %s", rec.Code, rec.Body.String())
	}

	now := time.Now().UTC()
	for i, cluster := range []*dbrepo.Cluster{clusterA, clusterB} {
		event := &dbrepo.AdmissionEvent{EventID: "scope-event-" + itoa(int64(i)), ClusterID: cluster.ID, ClusterName: cluster.Name, OccurredAt: now, Verdict: "audit", Resource: "pods"}
		if err := repos.Admission.CreateEvent(t.Context(), event); err != nil {
			t.Fatal(err)
		}
	}
	eventsRec := authRequest(t, h, http.MethodGet, "/api/v1/admission-events", nil, viewerCookie, false)
	events := responseData[dto.AdmissionEventPageView](t, eventsRec.Body.Bytes())
	if eventsRec.Code != http.StatusOK || events.Total != 1 || events.Items[0].ClusterName != "cluster-a" {
		t.Fatalf("tenant admission events = %d %+v", eventsRec.Code, events)
	}
	var otherIncidentID int64
	for i, cluster := range []*dbrepo.Cluster{clusterA, clusterB} {
		incident := admissionobs.Incident{IncidentKey: "scope-incident-" + itoa(int64(i)), ReporterInstanceID: "operator-0", Revision: 1, Component: admissionobs.ComponentOperator, Reason: admissionobs.ReasonQueueFull, StartedAt: now, LastSeenAt: now, AffectedCount: 1, SampleEventIDs: []string{"scope-event-" + itoa(int64(i))}}
		if err := repos.Incidents.Upsert(t.Context(), cluster.ID, cluster.Name, incident, now); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			rows, _, err := repos.Incidents.ListPage(t.Context(), dbrepo.IncidentPageOptions{ClusterNames: []string{cluster.Name}, PageOptions: dbrepo.PageOptions{Limit: 10, SortBy: "lastSeenAt"}})
			if err != nil || len(rows) != 1 {
				t.Fatalf("find other incident = %+v, %v", rows, err)
			}
			otherIncidentID = rows[0].ID
		}
	}
	incidentsRec := authRequest(t, h, http.MethodGet, "/api/v1/admission-link-incidents", nil, viewerCookie, false)
	incidents := responseData[dto.IncidentPageView](t, incidentsRec.Body.Bytes())
	if incidentsRec.Code != http.StatusOK || incidents.Total != 1 || incidents.Items[0].ClusterName != "cluster-a" {
		t.Fatalf("tenant incidents = %d %+v", incidentsRec.Code, incidents)
	}
	if rec := authRequest(t, h, http.MethodGet, "/api/v1/admission-link-incidents/"+itoa(otherIncidentID), nil, viewerCookie, false); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant incident = %d %s", rec.Code, rec.Body.String())
	}

	distribution := &dbrepo.PolicyDistribution{Name: "tenant-target", TenantID: &first.ID, ResourceOwnerType: "tenant", Spec: datatypes.NewJSONType(gatewaymodel.PolicyDistributionSpec{Clusters: gatewaymodel.DistributionClusters{ByName: []string{"cluster-a"}}, PolicyGroups: []string{"default"}})}
	if err := repos.Distributions.Create(t.Context(), distribution); err != nil {
		t.Fatal(err)
	}
	transfer := authRequest(t, h, http.MethodPut, "/api/v1/clusters/cluster-a/tenant", dto.ClusterTenantRequest{TenantID: &second.ID}, adminCookie, true)
	if transfer.Code != http.StatusConflict {
		t.Fatalf("targeted cluster transfer = %d %s", transfer.Code, transfer.Body.String())
	}
	assign := authRequest(t, h, http.MethodPut, "/api/v1/clusters/unassigned/tenant", dto.ClusterTenantRequest{TenantID: &first.ID}, adminCookie, true)
	if assign.Code != http.StatusOK {
		t.Fatalf("cluster assignment = %d %s", assign.Code, assign.Body.String())
	}
}
