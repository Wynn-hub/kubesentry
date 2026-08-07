package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
)

// fakePolicyRepo is an in-memory PolicyRepository. It exists so the service
// layer's business rules are testable without a database — the exact reason
// the interface was split out of dbrepo.
type fakePolicyRepo struct {
	byName   map[string]*dbrepo.Policy
	versions map[int64][]dbrepo.PolicyVersion
	nextID   int64
}

func newFakePolicyRepo() *fakePolicyRepo {
	return &fakePolicyRepo{
		byName:   map[string]*dbrepo.Policy{},
		versions: map[int64][]dbrepo.PolicyVersion{},
		nextID:   1,
	}
}

func (f *fakePolicyRepo) ListPage(ctx context.Context, opts dbrepo.PolicyPageOptions) ([]dbrepo.Policy, int64, error) {
	all := make([]dbrepo.Policy, 0, len(f.byName))
	for _, p := range f.byName {
		all = append(all, *p)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })
	return all, int64(len(all)), nil
}

func (f *fakePolicyRepo) List(ctx context.Context) ([]dbrepo.Policy, error) {
	all, _, err := f.ListPage(ctx, dbrepo.PolicyPageOptions{})
	return all, err
}

func (f *fakePolicyRepo) GetByName(ctx context.Context, name string) (*dbrepo.Policy, error) {
	p, ok := f.byName[name]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return p, nil
}

func (f *fakePolicyRepo) CreateWithVersion(ctx context.Context, p *dbrepo.Policy, v *dbrepo.PolicyVersion) error {
	if _, dup := f.byName[p.Name]; dup {
		return errors.New("duplicate key value violates unique constraint (SQLSTATE 23505)")
	}
	p.ID = f.nextID
	f.nextID++
	p.UpdatedAt = time.Now()
	f.byName[p.Name] = p
	v.PolicyID = p.ID
	f.versions[p.ID] = append(f.versions[p.ID], *v)
	return nil
}

func (f *fakePolicyRepo) UpdateWithVersion(ctx context.Context, id int64, expectedUpdatedAt time.Time, fields map[string]any, v *dbrepo.PolicyVersion) error {
	for _, p := range f.byName {
		if p.ID == id {
			if !p.UpdatedAt.Equal(expectedUpdatedAt) {
				return dbrepo.ErrConflict
			}
			p.UpdatedAt = time.Now()
			// Apply fields
			for k, val := range fields {
				switch k {
				case "match":
					p.Match = val.(datatypes.JSONType[v1alpha1.PolicyMatch])
				case "enforcement_mode":
					p.EnforcementMode = val.(string)
				case "rego":
					p.Rego = val.(string)
				case "description":
					p.Description = val.(string)
				case "compile_ok":
					p.CompileOK = val.(bool)
				case "compile_error":
					p.CompileError = val.(string)
				case "labels":
					p.Labels = val.(datatypes.JSONType[map[string]string])
				case "annotations":
					p.Annotations = val.(datatypes.JSONType[map[string]string])
				}
			}
			v.PolicyID = id
			f.versions[id] = append(f.versions[id], *v)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (f *fakePolicyRepo) Delete(ctx context.Context, id int64) error {
	for name, p := range f.byName {
		if p.ID == id {
			delete(f.byName, name)
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (f *fakePolicyRepo) ListVersions(ctx context.Context, policyID int64) ([]dbrepo.PolicyVersion, error) {
	return f.versions[policyID], nil
}

// fakeGroupRepo returns no groups by default — enough for most tests; a test
// needing references seeds it directly.
type fakeGroupRepo struct{ groups []dbrepo.PolicyGroup }

func (f *fakeGroupRepo) List(ctx context.Context) ([]dbrepo.PolicyGroup, error) {
	return f.groups, nil
}

// recordingNotifier captures NotifyChange calls.
type recordingNotifier struct{ topics []string }

func (n *recordingNotifier) NotifyChange(ctx context.Context, topic string) {
	n.topics = append(n.topics, topic)
}

const testRego = "package kubesentry\n\ndeny[msg] {\n\tfalse\n\tmsg := \"test\"\n}"

func policyRequestFixture() *dto.PolicyRequest {
	name := dto.OptString{Value: "test-policy", Set: true}
	desc := dto.OptString{Value: "test description", Set: true}
	return &dto.PolicyRequest{
		Name:            name,
		Description:     desc,
		EnforcementMode: dto.PolicyRequestEnforcementModeAudit,
		Match: dto.PolicyMatch{
			Operations: []dto.PolicyMatchOperationsItem{dto.PolicyMatchOperationsItemCREATE},
			Resources: []dto.MatchResource{
				{ApiGroups: []string{""}, ApiVersions: []string{"v1"}, Resources: []string{"pods"}},
			},
		},
		Rego: testRego,
	}
}

func TestCreateNotifiesAndStoresVersion(t *testing.T) {
	repo := newFakePolicyRepo()
	notifier := &recordingNotifier{}
	s := NewPolicyService(repo, &fakeGroupRepo{}, notifier)

	if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(notifier.topics) != 1 || notifier.topics[0] != "policy" {
		t.Fatalf("notifier topics = %v, want [policy]", notifier.topics)
	}
	if len(repo.versions) != 1 {
		t.Fatalf("no version snapshot stored: %v", repo.versions)
	}
}

func TestCreateDuplicateNameConflict(t *testing.T) {
	repo := newFakePolicyRepo()
	s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
	if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	err := s.Create(context.Background(), policyRequestFixture())
	if err == nil {
		t.Fatal("second Create: expected conflict error")
	}
	if !errors.Is(err, dbrepo.ErrConflict) {
		t.Fatalf("err = %v, want errors.Is ErrConflict", err)
	}
}

func TestCreateInvalidRegoRejectedBeforeStore(t *testing.T) {
	repo := newFakePolicyRepo()
	s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
	req := policyRequestFixture()
	req.Rego = "package wrong\n\nthis is not valid rego at all [[["
	err := s.Create(context.Background(), req)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v (%T), want ValidationError", err, err)
	}
	if len(repo.byName) != 0 {
		t.Fatal("invalid rego must not reach the store")
	}
}

func TestCreateMissingNameValidation(t *testing.T) {
	s := NewPolicyService(newFakePolicyRepo(), &fakeGroupRepo{}, nil)
	req := policyRequestFixture()
	req.Name = dto.OptString{}
	var ve *ValidationError
	if err := s.Create(context.Background(), req); !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError for missing name", err)
	}
}

func TestGetNotFound(t *testing.T) {
	s := NewPolicyService(newFakePolicyRepo(), &fakeGroupRepo{}, nil)
	_, err := s.Get(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateStaleResourceVersion(t *testing.T) {
	repo := newFakePolicyRepo()
	s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
	if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	req := policyRequestFixture()
	req.ResourceVersion = dto.OptString{Value: "999999999999", Set: true}
	err := s.Update(context.Background(), "test-policy", req)
	if !errors.Is(err, dbrepo.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestDeleteNotFound(t *testing.T) {
	s := NewPolicyService(newFakePolicyRepo(), &fakeGroupRepo{}, nil)
	if err := s.Delete(context.Background(), "nope", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRollbackNextAlwaysRejected(t *testing.T) {
	repo := newFakePolicyRepo()
	s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
	// Seed a policy so it exists
	if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := s.Rollback(context.Background(), "test-policy", "next")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v (%T), want ValidationError", err, err)
	}
}

func TestValidateRego(t *testing.T) {
	s := NewPolicyService(nil, nil, nil)
	if err := s.ValidateRego(context.Background(), testRego); err != nil {
		t.Fatalf("valid rego rejected: %v", err)
	}
	var ve *ValidationError
	if err := s.ValidateRego(context.Background(), "not rego"); !errors.As(err, &ve) {
		t.Fatalf("invalid rego: err = %v, want ValidationError", err)
	}
}

// referencedBy 为空时必须返回空切片而不是 nil（P2-CF1 在 service 层的护栏）。
func TestGetReferencedByEmptyNotNil(t *testing.T) {
	repo := newFakePolicyRepo()
	s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
	if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	d, err := s.Get(context.Background(), "test-policy")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if d.ReferencedBy == nil {
		t.Fatal("ReferencedBy is nil, want empty slice — nil would serialize as null")
	}
}
