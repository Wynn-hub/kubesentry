package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
)

// ManagedObject is one CR in a rendered snapshot. Spec is the canonical
// JSON of the v1alpha1 spec struct (deterministic: struct field order +
// json.Marshal's sorted map keys).
type ManagedObject struct {
	Kind   string // "Policy" | "PolicyGroup"
	Name   string
	Labels map[string]string
	Spec   []byte
}

// RenderResult is a self-contained snapshot for one cluster.
type RenderResult struct {
	Hash    string            // sha256 over the sorted object list; never empty
	Objects []ManagedObject   // policies first, then groups, each name-sorted
	Matched []string          // distributions targeting the cluster (sorted)
	Errors  map[string]string // distribution name -> render failure
}

// Render computes the snapshot from in-memory lists only — a pure function.
// A distribution with a bad group reference contributes an Errors entry but
// never blocks other distributions.
func Render(cluster *v1alpha1.Cluster, dists []v1alpha1.PolicyDistribution, groups []v1alpha1.PolicyGroup, policies []v1alpha1.Policy) RenderResult {
	res := RenderResult{Errors: map[string]string{}}

	groupByName := make(map[string]*v1alpha1.PolicyGroup, len(groups))
	for i := range groups {
		groupByName[groups[i].Name] = &groups[i]
	}
	policyByName := make(map[string]*v1alpha1.Policy, len(policies))
	for i := range policies {
		policyByName[policies[i].Name] = &policies[i]
	}

	type renderedGroup struct {
		src   *v1alpha1.PolicyGroup
		modes map[string]string // policy name -> strictest effective mode
	}
	rendered := map[string]*renderedGroup{}
	needed := map[string]*v1alpha1.Policy{}

	for i := range dists {
		d := &dists[i]
		if d.DeletionTimestamp != nil {
			continue
		}
		matched, matchErr := distMatches(d, cluster)
		if matchErr != "" {
			// Intentionally included in Matched despite the error: Task 6's
			// status reconciliation iterates Matched to decide per-cluster
			// status (Error/Pending/removed), and an errored-but-targeting
			// distribution must get an Error entry, not be silently dropped
			// as "not targeting this cluster".
			res.Matched = append(res.Matched, d.Name)
			res.Errors[d.Name] = matchErr
			continue
		}
		if !matched {
			continue
		}
		res.Matched = append(res.Matched, d.Name)

		override := ""
		for _, o := range d.Spec.Overrides {
			if o.Cluster == cluster.Name {
				override = o.EnforcementMode
			}
		}

		var errs []string
		for _, gname := range d.Spec.PolicyGroups {
			g, ok := groupByName[gname]
			if !ok {
				errs = append(errs, fmt.Sprintf("policygroup %q not found", gname))
				continue
			}
			if !g.Spec.Enabled {
				// Matches PolicyGroupReconciler: a disabled group resolves to
				// zero members, not an error — deliberate admin no-op.
				continue
			}
			members, _ := groupresolve.Resolve(g.Spec, policies)
			rg := rendered[gname]
			if rg == nil {
				rg = &renderedGroup{src: g, modes: map[string]string{}}
				rendered[gname] = rg
			}
			for _, m := range members {
				mode := m.EnforcementMode
				if override != "" {
					mode = override
				}
				rg.modes[m.Name] = stricter(rg.modes[m.Name], mode)
				if p, ok := policyByName[m.Name]; ok {
					needed[m.Name] = p
				}
			}
		}
		if len(errs) > 0 {
			res.Errors[d.Name] = strings.Join(errs, "; ")
		}
	}
	sort.Strings(res.Matched)

	for _, name := range sortedKeys(needed) {
		p := needed[name]
		spec, _ := json.Marshal(p.Spec)
		res.Objects = append(res.Objects, ManagedObject{
			Kind: "Policy", Name: name, Labels: withManaged(p.Labels), Spec: spec,
		})
	}
	for _, gname := range sortedKeys(rendered) {
		rg := rendered[gname]
		// ponytail: PolicyGroupSpec has no generated DeepCopy(), only
		// DeepCopyInto — build the snapshot spec through that instead of
		// the brief's Spec.DeepCopy() call, which doesn't exist.
		var spec v1alpha1.PolicyGroupSpec
		rg.src.Spec.DeepCopyInto(&spec)
		refs := make([]v1alpha1.PolicyRef, 0, len(rg.modes))
		for _, pname := range sortedKeys(rg.modes) {
			refs = append(refs, v1alpha1.PolicyRef{Name: pname, EnforcementMode: rg.modes[pname]})
		}
		spec.Policies = v1alpha1.PolicyGroupPolicies{ByName: refs}
		spec.SelectorEnforcementMode = ""
		raw, _ := json.Marshal(spec)
		res.Objects = append(res.Objects, ManagedObject{
			Kind: "PolicyGroup", Name: gname, Labels: withManaged(rg.src.Labels), Spec: raw,
		})
	}

	sum, _ := json.Marshal(res.Objects)
	h := sha256.Sum256(sum)
	res.Hash = hex.EncodeToString(h[:])
	return res
}

// distMatches: byName ∪ clusterSelector (against Cluster.spec.labels).
// A malformed selector yields a non-empty error string.
func distMatches(d *v1alpha1.PolicyDistribution, cluster *v1alpha1.Cluster) (bool, string) {
	for _, n := range d.Spec.Clusters.ByName {
		if n == cluster.Name {
			return true, ""
		}
	}
	if d.Spec.Clusters.ClusterSelector == nil {
		return false, ""
	}
	sel, err := metav1.LabelSelectorAsSelector(d.Spec.Clusters.ClusterSelector)
	if err != nil {
		return false, fmt.Sprintf("invalid clusterSelector: %v", err)
	}
	return sel.Matches(labels.Set(cluster.Spec.Labels)), ""
}

func stricter(a, b string) string {
	if a == v1alpha1.ModeEnforce || b == v1alpha1.ModeEnforce {
		return v1alpha1.ModeEnforce
	}
	if a == "" {
		return b
	}
	return a
}

func withManaged(in map[string]string) map[string]string {
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	out[v1alpha1.ManagedByLabel] = v1alpha1.ManagedByPlatform
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ToProto converts snapshot objects for the wire.
func ToProto(objs []ManagedObject) []*gatewaypb.ManagedObject {
	out := make([]*gatewaypb.ManagedObject, 0, len(objs))
	for _, o := range objs {
		out = append(out, &gatewaypb.ManagedObject{
			Kind: o.Kind, Name: o.Name, Labels: o.Labels, SpecJson: o.Spec,
		})
	}
	return out
}
