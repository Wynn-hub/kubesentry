package hublink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/gatewaypb"
)

// applyDesiredState upserts snapshot objects (policies before groups) and
// prunes managed objects absent from the snapshot (groups before policies).
// Unmanaged same-name objects: identical spec = satisfied (self-registration
// shares one store with the platform library — never adopt, never label),
// different spec = per-object error, never overwritten. Errors are
// aggregated so one bad object doesn't block the rest.
func applyDesiredState(ctx context.Context, c client.Client, ds *gatewaypb.DesiredState) error {
	var errs []string
	inSnapshot := map[string]map[string]bool{"Policy": {}, "PolicyGroup": {}}

	var valid []*gatewaypb.ManagedObject
	for _, obj := range ds.Objects {
		if _, known := inSnapshot[obj.Kind]; !known {
			errs = append(errs, fmt.Sprintf("unknown kind %q for object %q", obj.Kind, obj.Name))
			continue
		}
		valid = append(valid, obj)
	}

	for _, kind := range []string{"Policy", "PolicyGroup"} {
		for _, obj := range valid {
			if obj.Kind != kind {
				continue
			}
			inSnapshot[obj.Kind][obj.Name] = true
			if err := upsertObject(ctx, c, obj); err != nil {
				errs = append(errs, fmt.Sprintf("%s %s: %v", obj.Kind, obj.Name, err))
			}
		}
	}

	if err := pruneManaged(ctx, c, inSnapshot); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// groupSpecsEqual compares two PolicyGroupSpecs treating Policies.ByName as
// an unordered set: the hub always renders it name-sorted (render.go's
// sortedKeys), but a group's live spec (e.g. a chart-authored library
// object) may list members in an arbitrary source order. Plain
// reflect.DeepEqual on the slice would report a spurious mismatch from
// order alone even when membership and modes are identical.
func groupSpecsEqual(a, b v1alpha1.PolicyGroupSpec) bool {
	if !reflect.DeepEqual(sortedByName(a.Policies.ByName), sortedByName(b.Policies.ByName)) {
		return false
	}
	a.Policies.ByName, b.Policies.ByName = nil, nil
	return reflect.DeepEqual(a, b)
}

func sortedByName(refs []v1alpha1.PolicyRef) []v1alpha1.PolicyRef {
	out := append([]v1alpha1.PolicyRef(nil), refs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func upsertObject(ctx context.Context, c client.Client, obj *gatewaypb.ManagedObject) error {
	switch obj.Kind {
	case "Policy":
		var spec v1alpha1.PolicySpec
		if err := json.Unmarshal(obj.SpecJson, &spec); err != nil {
			return fmt.Errorf("decode spec: %w", err)
		}
		var existing v1alpha1.Policy
		err := c.Get(ctx, types.NamespacedName{Name: obj.Name}, &existing)
		if apierrors.IsNotFound(err) {
			return c.Create(ctx, &v1alpha1.Policy{
				ObjectMeta: metav1.ObjectMeta{Name: obj.Name, Labels: obj.Labels},
				Spec:       spec,
			})
		}
		if err != nil {
			return err
		}
		if existing.Labels[v1alpha1.ManagedByLabel] == "" {
			if reflect.DeepEqual(existing.Spec, spec) {
				return nil // unmanaged twin with identical content: satisfied
			}
			return errors.New("conflicts with existing unmanaged object")
		}
		if reflect.DeepEqual(existing.Spec, spec) && reflect.DeepEqual(existing.Labels, obj.Labels) {
			return nil
		}
		existing.Spec = spec
		existing.Labels = obj.Labels
		return c.Update(ctx, &existing)
	case "PolicyGroup":
		var spec v1alpha1.PolicyGroupSpec
		if err := json.Unmarshal(obj.SpecJson, &spec); err != nil {
			return fmt.Errorf("decode spec: %w", err)
		}
		var existing v1alpha1.PolicyGroup
		err := c.Get(ctx, types.NamespacedName{Name: obj.Name}, &existing)
		if apierrors.IsNotFound(err) {
			return c.Create(ctx, &v1alpha1.PolicyGroup{
				ObjectMeta: metav1.ObjectMeta{Name: obj.Name, Labels: obj.Labels},
				Spec:       spec,
			})
		}
		if err != nil {
			return err
		}
		if existing.Labels[v1alpha1.ManagedByLabel] == "" {
			if groupSpecsEqual(existing.Spec, spec) {
				return nil
			}
			return errors.New("conflicts with existing unmanaged object")
		}
		if groupSpecsEqual(existing.Spec, spec) && reflect.DeepEqual(existing.Labels, obj.Labels) {
			return nil
		}
		existing.Spec = spec
		existing.Labels = obj.Labels
		return c.Update(ctx, &existing)
	default:
		return fmt.Errorf("unknown kind %q", obj.Kind)
	}
}

// pruneManaged deletes labeled objects missing from the snapshot — groups
// first so no group ever references a policy deleted moments earlier.
func pruneManaged(ctx context.Context, c client.Client, inSnapshot map[string]map[string]bool) error {
	var errs []string
	sel := client.HasLabels{v1alpha1.ManagedByLabel}

	var groups v1alpha1.PolicyGroupList
	if err := c.List(ctx, &groups, sel); err != nil {
		return fmt.Errorf("list managed groups: %w", err)
	}
	for i := range groups.Items {
		g := &groups.Items[i]
		if inSnapshot["PolicyGroup"][g.Name] {
			continue
		}
		if err := c.Delete(ctx, g); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Sprintf("prune PolicyGroup %s: %v", g.Name, err))
		}
	}

	var policies v1alpha1.PolicyList
	if err := c.List(ctx, &policies, sel); err != nil {
		return fmt.Errorf("list managed policies: %w", err)
	}
	for i := range policies.Items {
		p := &policies.Items[i]
		if inSnapshot["Policy"][p.Name] {
			continue
		}
		if err := c.Delete(ctx, p); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Sprintf("prune Policy %s: %v", p.Name, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// policyCounts tallies member policies by phase for StatusReport.
func policyCounts(ctx context.Context, c client.Client) (ready, invalid int32, err error) {
	var list v1alpha1.PolicyList
	if err := c.List(ctx, &list); err != nil {
		return 0, 0, err
	}
	for _, p := range list.Items {
		switch p.Status.Phase {
		case v1alpha1.PhaseReady:
			ready++
		case v1alpha1.PhaseInvalid:
			invalid++
		}
	}
	return ready, invalid, nil
}
