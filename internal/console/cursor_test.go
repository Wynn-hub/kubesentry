package console

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
)

func policyWithCursor(current int64, ann string) *v1alpha1.Policy {
	p := &v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{Name: "p"},
		Status:     v1alpha1.PolicyStatus{CurrentVersion: current},
	}
	if ann != "" {
		p.Annotations = map[string]string{cursorAnnotation: ann}
	}
	return p
}

func TestResolveCursor(t *testing.T) {
	cases := []struct {
		name       string
		current    int64
		ann        string
		wantCursor int64
		wantHead   int64
		wantFlight bool
	}{
		{"no annotation → at head", 3, "", 3, 3, false},
		{"settled rollback", 3, `{"cursor":1,"atVersion":2,"head":2}`, 1, 2, false},
		{"in-flight rollback", 2, `{"cursor":1,"atVersion":2,"head":2}`, 1, 2, true},
		{"stale (edited outside console)", 5, `{"cursor":1,"atVersion":2,"head":2}`, 5, 5, false},
		{"garbage annotation", 4, `not-json`, 4, 4, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, hd, f := resolveCursor(policyWithCursor(tc.current, tc.ann))
			if c != tc.wantCursor || hd != tc.wantHead || f != tc.wantFlight {
				t.Fatalf("got (%d,%d,%v), want (%d,%d,%v)", c, hd, f, tc.wantCursor, tc.wantHead, tc.wantFlight)
			}
		})
	}
}
