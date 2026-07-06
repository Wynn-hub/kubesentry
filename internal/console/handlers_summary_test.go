package console

import "testing"

func TestSummary(t *testing.T) {
	h, _ := newTestServer(t,
		testPolicy("p1", "builtin", "enforce", "Ready"),
		testPolicy("p2", "custom", "audit", "Invalid"),
		testGroup("g1", true),
	)
	rec, env := doRequest(t, h, "GET", "/api/v1/summary", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	s := mustUnmarshal[summaryData](t, env.Data)
	if s.Totals["policies"] != 2 || s.Totals["policygroups"] != 1 || s.Totals["exceptions"] != 0 {
		t.Fatalf("totals = %+v", s.Totals)
	}
	if s.PolicyPhases["Ready"] != 1 || s.PolicyPhases["Invalid"] != 1 {
		t.Fatalf("policyPhases = %+v", s.PolicyPhases)
	}
	if s.PolicyModes["enforce"] != 1 || s.PolicyModes["audit"] != 1 {
		t.Fatalf("policyModes = %+v", s.PolicyModes)
	}
}
