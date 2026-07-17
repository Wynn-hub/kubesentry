// Command gen-seed-migration converts the existing
// charts/kubesentry-hub/builtin-policies/ source yaml into the SQL INSERT
// statements for migration 0002 (spec D9). Run once, review the output,
// paste it into 0002_seed_builtin_policies.up.sql — not run at deploy time.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Wynn-hub/kubesentry/internal/dbmodel"
)

type policyYAML struct {
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Description     string `yaml:"description"`
		EnforcementMode string `yaml:"enforcementMode"`
		Rego            string `yaml:"rego"`
		Match           struct {
			Operations []string `yaml:"operations"`
			Resources  []struct {
				APIGroups   []string `yaml:"apiGroups"`
				APIVersions []string `yaml:"apiVersions"`
				Resources   []string `yaml:"resources"`
			} `yaml:"resources"`
		} `yaml:"match"`
	} `yaml:"spec"`
}

// matchResource/matchSpec/groupMember/groupPolicies/groupSpec mirror the
// JSONB shape stored in policies.match / policy_groups.spec. Marshaled via
// encoding/json so string values get correct JSON (double-quote) escaping —
// sqlQuote below is for SQL string-literal escaping only and must never be
// used to build JSON content directly (single-quoting a JSON value produces
// invalid JSON that ::jsonb rejects).
type matchResource struct {
	APIGroups   []string `json:"apiGroups"`
	APIVersions []string `json:"apiVersions"`
	Resources   []string `json:"resources"`
}

type matchSpec struct {
	Operations []string        `json:"operations"`
	Resources  []matchResource `json:"resources"`
}

type groupMember struct {
	Name            string `json:"name"`
	EnforcementMode string `json:"enforcementMode"`
}

type groupSpec struct {
	DisplayName string `json:"displayName"`
	Enabled     bool   `json:"enabled"`
	Policies    struct {
		ByName []groupMember `json:"byName"`
	} `json:"policies"`
}

// sqlQuote escapes s for use as a single-quoted SQL string literal. Not for
// building JSON content — see the type comment above.
func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return string(b)
}

func main() {
	// ponytail: throwaway node id, this binary runs once on a laptop, not in prod.
	if err := dbmodel.Init(1); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	dir := "charts/kubesentry-hub/builtin-policies/policies"
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var names []string
	policies := map[string]policyYAML{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var p policyYAML
		if err := yaml.Unmarshal(raw, &p); err != nil {
			fmt.Fprintln(os.Stderr, e.Name(), err)
			os.Exit(1)
		}
		names = append(names, p.Metadata.Name)
		policies[p.Metadata.Name] = p
	}
	sort.Strings(names)

	fmt.Println("SET search_path TO kubesentry;")
	fmt.Println()
	var groupMembers []groupMember
	for _, name := range names {
		p := policies[name]
		matchJSON := mustJSON(matchSpec{
			Operations: p.Spec.Match.Operations,
			Resources: []matchResource{{
				APIGroups:   p.Spec.Match.Resources[0].APIGroups,
				APIVersions: p.Spec.Match.Resources[0].APIVersions,
				Resources:   p.Spec.Match.Resources[0].Resources,
			}},
		})
		fmt.Printf("INSERT INTO policies (id, name, description, rego, enforcement_mode, match, compile_ok) VALUES (%d, %s, %s, %s, %s, %s::jsonb, true);\n",
			dbmodel.NewID(),
			sqlQuote(name), sqlQuote(p.Spec.Description), sqlQuote(p.Spec.Rego), sqlQuote(p.Spec.EnforcementMode), sqlQuote(matchJSON))
		groupMembers = append(groupMembers, groupMember{Name: name, EnforcementMode: p.Spec.EnforcementMode})
	}
	fmt.Println()
	gs := groupSpec{DisplayName: "Default", Enabled: true}
	gs.Policies.ByName = groupMembers
	fmt.Printf("INSERT INTO policy_groups (id, name, spec) VALUES (%d, 'default', %s::jsonb);\n", dbmodel.NewID(), sqlQuote(mustJSON(gs)))

	distSpec := map[string]interface{}{
		"policyGroups": []string{"default"},
		"clusters": map[string]interface{}{
			"clusterSelector": map[string]interface{}{},
		},
	}
	fmt.Printf("INSERT INTO policy_distributions (id, name, spec) VALUES (%d, 'default', %s::jsonb);\n", dbmodel.NewID(), sqlQuote(mustJSON(distSpec)))
}
