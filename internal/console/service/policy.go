package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/console/repository"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/groupresolve"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

const visualBuilderSpecAnnotation = "kubesentry.io/visual-builder-spec"

// PolicyService owns the policy domain's business logic.
type PolicyService struct {
	policies repository.PolicyRepository
	groups   repository.GroupRepository
	notifier Notifier
}

func NewPolicyService(
	policies repository.PolicyRepository,
	groups repository.GroupRepository,
	notifier Notifier,
) *PolicyService {
	return &PolicyService{
		policies: policies,
		groups:   groups,
		notifier: notifier,
	}
}

// ListOptions carries the parsed list query from the handler into the service.
// Pagination validation (pageSize in {10,20,50,100}, sortBy whitelist) happens
// in the handler — the service trusts these values.
type ListOptions struct {
	Page, PageSize         int
	SortBy, SortOrder      string
	Phases, Modes, Sources []string
	Keyword                string
}

func (s *PolicyService) List(ctx context.Context, opts ListOptions) (*dto.PolicyListData, error) {
	policies, total, err := s.policies.ListPage(ctx, dbrepo.PolicyPageOptions{
		PageOptions: dbrepo.PageOptions{
			Limit:  opts.PageSize,
			Offset: (opts.Page - 1) * opts.PageSize,
			SortBy: opts.SortBy,
			Desc:   opts.SortOrder == "desc",
		},
		Phases:  opts.Phases,
		Modes:   opts.Modes,
		Sources: opts.Sources,
		Keyword: opts.Keyword,
	})
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}

	items := make([]dto.PolicyListItem, 0, len(policies))
	for i := range policies {
		items = append(items, dto.PolicyToListItem(&policies[i]))
	}

	return &dto.PolicyListData{
		Items:    items,
		Total:    int(total),
		Page:     opts.Page,
		PageSize: dto.PolicyListDataPageSize(opts.PageSize),
	}, nil
}

func (s *PolicyService) Get(ctx context.Context, name string) (*dto.PolicyDetail, error) {
	p, err := s.policies.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get policy: %w", err)
	}

	refs, err := s.referencingGroups(ctx, p.Name)
	if err != nil {
		return nil, fmt.Errorf("list policy groups: %w", err)
	}

	detail := dto.PolicyToDetail(p, refs)
	return &detail, nil
}

func (s *PolicyService) Create(ctx context.Context, req *dto.PolicyRequest) error {
	if err := s.validatePolicyRequest(req, true); err != nil {
		return err
	}

	p := &dbrepo.Policy{
		Name:            req.Name.Or(""),
		Description:     req.Description.Or(""),
		Rego:            req.Rego,
		EnforcementMode: string(req.EnforcementMode),
		Match:           datatypes.NewJSONType(dto.MatchFromDTO(req.Match)),
		CompileOK:       true,
		// Or(nil) preserves the legacy handler's behavior exactly: an absent
		// labels field persisted a nil map, not an empty object.
		Labels:      datatypes.NewJSONType(map[string]string(req.Labels.Or(nil))),
		Annotations: datatypes.NewJSONType(applyAnnotations(nil, req.Annotations.Or(nil))),
	}

	err := s.policies.CreateWithVersion(ctx, p, &dbrepo.PolicyVersion{
		PolicyID:     p.ID,
		Version:      1,
		Rego:         p.Rego,
		SpecSnapshot: datatypes.NewJSONType(dto.PolicySpecSnapshot(p)),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("policy %s already exists: %w", req.Name.Or(""), dbrepo.ErrConflict)
		}
		return fmt.Errorf("create policy: %w", err)
	}

	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "policy")
	}
	return nil
}

func (s *PolicyService) Update(ctx context.Context, name string, req *dto.PolicyRequest) error {
	if err := s.validatePolicyRequest(req, false); err != nil {
		return err
	}

	p, err := s.policies.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("get policy: %w", err)
	}

	if rv, ok := req.ResourceVersion.Get(); ok && rv != "" && rv != dto.PolicyResourceVersion(p) {
		return fmt.Errorf("policy has been modified, refresh and retry: %w", dbrepo.ErrConflict)
	}

	expectedUpdatedAt := p.UpdatedAt

	lbl := p.Labels.Data()
	if reqLabels, ok := req.Labels.Get(); ok {
		if lbl == nil {
			lbl = map[string]string{}
		}
		for k, v := range reqLabels {
			lbl[k] = v
		}
	}

	ann := p.Annotations.Data()
	delete(ann, visualBuilderSpecAnnotation)
	if reqAnn, ok := req.Annotations.Get(); ok {
		ann = applyAnnotations(ann, reqAnn)
	}

	versions, err := s.policies.ListVersions(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("list versions: %w", err)
	}
	current := 0
	if len(versions) > 0 {
		current = versions[0].Version
	}

	p.Match = datatypes.NewJSONType(dto.MatchFromDTO(req.Match))
	p.EnforcementMode = string(req.EnforcementMode)
	p.Rego = req.Rego
	p.Description = req.Description.Or("")
	p.CompileOK = true
	p.CompileError = ""
	p.Labels = datatypes.NewJSONType(lbl)
	p.Annotations = datatypes.NewJSONType(ann)

	fields := map[string]any{
		"match":            p.Match,
		"enforcement_mode": p.EnforcementMode,
		"rego":             p.Rego,
		"description":      p.Description,
		"compile_ok":       p.CompileOK,
		"compile_error":    p.CompileError,
		"labels":           p.Labels,
		"annotations":      p.Annotations,
	}

	err = s.policies.UpdateWithVersion(ctx, p.ID, expectedUpdatedAt, fields, &dbrepo.PolicyVersion{
		PolicyID:     p.ID,
		Version:      current + 1,
		Rego:         p.Rego,
		SpecSnapshot: datatypes.NewJSONType(dto.PolicySpecSnapshot(p)),
	})
	if err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			return fmt.Errorf("policy has been modified, refresh and retry: %w", dbrepo.ErrConflict)
		}
		return fmt.Errorf("update policy: %w", err)
	}

	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "policy")
	}
	return nil
}

func (s *PolicyService) ValidateRego(ctx context.Context, rego string) error {
	if _, err := webhook.CompileRego(rego); err != nil {
		return &ValidationError{Msg: "rego compile failed: " + err.Error()}
	}
	return nil
}

func (s *PolicyService) Delete(ctx context.Context, name string, force bool) error {
	p, err := s.policies.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("get policy: %w", err)
	}

	if !force {
		refs, err := s.referencingGroups(ctx, p.Name)
		if err != nil {
			return fmt.Errorf("list policy groups: %w", err)
		}
		if len(refs) > 0 {
			return &ReferencedByError{Groups: refs}
		}
	}

	if err := s.policies.Delete(ctx, p.ID); err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}

	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "policy")
	}
	return nil
}

func (s *PolicyService) ListVersions(ctx context.Context, name string) (*dto.PolicyVersionList, error) {
	p, err := s.policies.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get policy: %w", err)
	}

	versions, err := s.policies.ListVersions(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}

	current := 0
	if len(versions) > 0 {
		current = versions[0].Version
	}

	entries := make([]dto.PolicyVersionEntry, 0, len(versions))
	for i := range versions {
		v := &versions[i]
		snap := v.SpecSnapshot.Data()
		entries = append(entries, dto.PolicyVersionEntry{
			Version:         v.Version,
			CreatedAt:       v.CreatedAt,
			Rego:            v.Rego,
			Match:           dto.MatchToDTO(snap.Match),
			EnforcementMode: dto.PolicyVersionEntryEnforcementMode(snap.EnforcementMode),
			IsCurrent:       v.Version == current,
		})
	}

	return &dto.PolicyVersionList{
		CurrentVersion: current,
		Versions:       entries,
	}, nil
}

func (s *PolicyService) Rollback(ctx context.Context, name string, direction string) (*dto.RollbackResult, error) {
	if direction != "prev" && direction != "next" {
		return nil, &ValidationError{Msg: `direction must be "prev" or "next"`}
	}

	p, err := s.policies.GetByName(ctx, name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get policy: %w", err)
	}

	if direction == "next" {
		return nil, &ValidationError{Msg: "already at the latest version"}
	}

	versions, err := s.policies.ListVersions(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}

	current := 0
	if len(versions) > 0 {
		current = versions[0].Version
	}

	target := current - 1
	if target < 1 {
		return nil, &ValidationError{Msg: "already at the oldest version"}
	}

	var targetVersion *dbrepo.PolicyVersion
	for i := range versions {
		if versions[i].Version == target {
			targetVersion = &versions[i]
			break
		}
	}
	if targetVersion == nil {
		return nil, fmt.Errorf("version %d snapshot has been pruned: %w", target, ErrGone)
	}

	expectedUpdatedAt := p.UpdatedAt
	snap := targetVersion.SpecSnapshot.Data()
	p.Rego = targetVersion.Rego
	p.Match = datatypes.NewJSONType(snap.Match)
	p.EnforcementMode = snap.EnforcementMode
	p.Description = snap.Description
	p.CompileOK = true
	p.CompileError = ""

	fields := map[string]any{
		"rego":             p.Rego,
		"match":            p.Match,
		"enforcement_mode": p.EnforcementMode,
		"description":      p.Description,
		"compile_ok":       p.CompileOK,
		"compile_error":    p.CompileError,
	}

	newVersion := current + 1
	err = s.policies.UpdateWithVersion(ctx, p.ID, expectedUpdatedAt, fields, &dbrepo.PolicyVersion{
		PolicyID:     p.ID,
		Version:      newVersion,
		Rego:         targetVersion.Rego,
		SpecSnapshot: targetVersion.SpecSnapshot,
	})
	if err != nil {
		if errors.Is(err, dbrepo.ErrConflict) {
			return nil, fmt.Errorf("policy has been modified, refresh and retry: %w", dbrepo.ErrConflict)
		}
		return nil, fmt.Errorf("update policy: %w", err)
	}

	if s.notifier != nil {
		s.notifier.NotifyChange(ctx, "policy")
	}

	return &dto.RollbackResult{
		RestoredFromVersion: target,
		NewVersion:          newVersion,
	}, nil
}

func (s *PolicyService) ResourceSuggestions(ctx context.Context) (*dto.ResourceSuggestions, error) {
	policies, err := s.policies.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}

	groups := map[string]struct{}{}
	versions := map[string]struct{}{}
	resources := map[string]struct{}{}
	resourcesByGroup := map[string]map[string]struct{}{}

	for i := range policies {
		match := policies[i].Match.Data()
		for _, res := range match.Resources {
			for _, g := range res.APIGroups {
				groups[g] = struct{}{}
				if resourcesByGroup[g] == nil {
					resourcesByGroup[g] = map[string]struct{}{}
				}
				for _, rsc := range res.Resources {
					resourcesByGroup[g][rsc] = struct{}{}
				}
			}
			for _, v := range res.APIVersions {
				versions[v] = struct{}{}
			}
			for _, rsc := range res.Resources {
				resources[rsc] = struct{}{}
			}
		}
	}

	byGroup := make(dto.ResourceSuggestionsResourcesByGroup, len(resourcesByGroup))
	for g, rsc := range resourcesByGroup {
		byGroup[g] = sortedKeys(rsc)
	}

	return &dto.ResourceSuggestions{
		ApiGroups:        sortedKeys(groups),
		ApiVersions:      sortedKeys(versions),
		Resources:        sortedKeys(resources),
		ResourcesByGroup: byGroup,
	}, nil
}

// validatePolicyRequest runs client-side validation before persistence.
func (s *PolicyService) validatePolicyRequest(req *dto.PolicyRequest, isCreate bool) error {
	if isCreate {
		name := req.Name.Or("")
		if name == "" {
			return &ValidationError{Msg: "name is required"}
		}
		if errs := validation.IsDNS1123Subdomain(name); len(errs) > 0 {
			return &ValidationError{Msg: "invalid name: " + strings.Join(errs, "; ")}
		}
	}

	mode := string(req.EnforcementMode)
	if mode != v1alpha1.ModeEnforce && mode != v1alpha1.ModeAudit {
		return &ValidationError{Msg: fmt.Sprintf("enforcementMode must be %q or %q", v1alpha1.ModeEnforce, v1alpha1.ModeAudit)}
	}

	if len(req.Match.Operations) == 0 {
		return &ValidationError{Msg: "match.operations must not be empty"}
	}
	if len(req.Match.Resources) == 0 {
		return &ValidationError{Msg: "match.resources must not be empty"}
	}

	if _, err := webhook.CompileRego(req.Rego); err != nil {
		return &ValidationError{Msg: "rego compile failed: " + err.Error()}
	}

	return nil
}

// applyAnnotations merges src into dst (lazy-initializing dst), treating
// empty-string values as "do not set" — this is how the visual builder
// clears its annotation on save.
func applyAnnotations(dst map[string]string, src map[string]string) map[string]string {
	for k, v := range src {
		if v == "" {
			continue
		}
		if dst == nil {
			dst = map[string]string{}
		}
		dst[k] = v
	}
	return dst
}

// referencingGroups returns the names of PolicyGroups that resolve
// policyName as an effective member (byName or bySelector, minus exclude).
func (s *PolicyService) referencingGroups(ctx context.Context, policyName string) ([]string, error) {
	groups, err := s.groups.List(ctx)
	if err != nil {
		return nil, err
	}
	policies, err := s.policies.List(ctx)
	if err != nil {
		return nil, err
	}

	resolvePolicies := make([]v1alpha1.Policy, len(policies))
	for i := range policies {
		resolvePolicies[i] = v1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:   policies[i].Name,
				Labels: policies[i].Labels.Data(),
			},
			Spec: v1alpha1.PolicySpec{
				EnforcementMode: policies[i].EnforcementMode,
			},
		}
	}

	var refs []string
	for i := range groups {
		members, _ := groupresolve.Resolve(groups[i].Spec.Data(), resolvePolicies)
		for _, m := range members {
			if m.Name == policyName {
				refs = append(refs, groups[i].Name)
				break
			}
		}
	}
	sort.Strings(refs)
	return refs, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505) — same check as the legacy console.isUniqueViolation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	// ponytail: fakes and non-pgx drivers only surface the code in the message.
	return strings.Contains(err.Error(), "SQLSTATE 23505")
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
