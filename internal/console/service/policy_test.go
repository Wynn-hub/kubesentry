package service

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/console/dto"
	dbrepo "github.com/Wynn-hub/kubesentry/internal/database/repository"
)

// fakePolicyRepo is an in-memory PolicyRepository. It exists so the service
// layer's business rules are testable without a database — the exact reason
// the interface was split out of dbrepo.
type fakePolicyRepo struct {
	byName   map[string]*dbrepo.Policy
	versions map[int64][]dbrepo.PolicyVersion
	nextID   int64
	// forceConflict makes UpdateWithVersion fail the compare-and-swap the way
	// a concurrent writer would, so the CAS error path is reachable in tests.
	forceConflict bool
	lockCalls     int
}

func (f *fakePolicyRepo) WithPolicyReferenceLock(_ context.Context, fn func() error) error {
	f.lockCalls++
	return fn()
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
		// A real *pgconn.PgError, not a message containing "23505": the
		// service detects duplicates via dbrepo.IsUniqueViolation's typed
		// errors.As check, so the fake must reproduce the real driver's error
		// shape or this path would silently stop being covered.
		return &pgconn.PgError{
			Code:    "23505",
			Message: `duplicate key value violates unique constraint "idx_policies_name"`,
		}
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
	if f.forceConflict {
		return dbrepo.ErrConflict
	}
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

// ListVersions returns newest-first, matching dbrepo.PolicyRepo's
// `Order("version DESC")`. The service reads versions[0] as the current
// version, so insertion order here would make it read the oldest instead.
func (f *fakePolicyRepo) ListVersions(ctx context.Context, policyID int64) ([]dbrepo.PolicyVersion, error) {
	out := append([]dbrepo.PolicyVersion(nil), f.versions[policyID]...)
	sort.Slice(out, func(i, j int) bool { return out[i].Version > out[j].Version })
	return out, nil
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

func policyRequestFixture() *dto.PolicyInput {
	name, desc := "test-policy", "test description"
	return &dto.PolicyInput{
		Name:            &name,
		Description:     &desc,
		EnforcementMode: "audit",
		Match: dto.PolicyMatchInput{
			Operations: []string{"CREATE"},
			Resources: []dto.MatchResourceInput{
				{APIGroups: []string{""}, APIVersions: []string{"v1"}, Resources: []string{"pods"}},
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

func TestPolicyMutationUsesReferenceLock(t *testing.T) {
	repo := newFakePolicyRepo()
	s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
	if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(context.Background(), "test-policy", policyRequestFixture()); err != nil {
		t.Fatal(err)
	}
	if repo.lockCalls != 1 {
		t.Fatalf("reference lock calls = %d, want 1", repo.lockCalls)
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

// Conflict messages must read exactly as the legacy handlers wrote them. An
// earlier revision built these with fmt.Errorf("...: %w", dbrepo.ErrConflict),
// which appended the sentinel's internal text ("dbrepo: row was modified since
// it was read") to every 409 body the API returns — leaking a package name and,
// for the duplicate-name case, describing an unrelated situation. These cases
// pin both halves of the contract: the sentinel still matches for the handler's
// status mapping, and the message stays clean for the user.
func TestConflictMessagesMatchLegacyHandlers(t *testing.T) {
	t.Run("duplicate name", func(t *testing.T) {
		repo := newFakePolicyRepo()
		s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
		if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
			t.Fatalf("seed: %v", err)
		}
		err := s.Create(context.Background(), policyRequestFixture())
		// handlers_policy.go:273
		const want = "policy test-policy already exists"
		if err == nil || err.Error() != want {
			t.Fatalf("err = %v, want exactly %q", err, want)
		}
		if !errors.Is(err, dbrepo.ErrConflict) {
			t.Fatalf("err = %v must still match dbrepo.ErrConflict", err)
		}
	})

	// handlers_policy.go:321 (stale resourceVersion), :381 (update CAS),
	// :593 (rollback CAS) all emit this one string.
	const wantModified = "policy has been modified, refresh and retry"

	t.Run("stale resourceVersion", func(t *testing.T) {
		repo := newFakePolicyRepo()
		s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
		if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
			t.Fatalf("seed: %v", err)
		}
		req := policyRequestFixture()
		resourceVersion := "999999999999"
		req.ResourceVersion = &resourceVersion
		err := s.Update(context.Background(), "test-policy", req)
		if err == nil || err.Error() != wantModified {
			t.Fatalf("err = %v, want exactly %q", err, wantModified)
		}
		if !errors.Is(err, dbrepo.ErrConflict) {
			t.Fatalf("err = %v must still match dbrepo.ErrConflict", err)
		}
	})

	t.Run("update CAS conflict", func(t *testing.T) {
		repo := newFakePolicyRepo()
		s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
		if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// Force the repo's compare-and-swap to fail the way a concurrent
		// writer would, without going through the resourceVersion pre-check.
		repo.forceConflict = true
		err := s.Update(context.Background(), "test-policy", policyRequestFixture())
		if err == nil || err.Error() != wantModified {
			t.Fatalf("err = %v, want exactly %q", err, wantModified)
		}
		if !errors.Is(err, dbrepo.ErrConflict) {
			t.Fatalf("err = %v must still match dbrepo.ErrConflict", err)
		}
	})

	t.Run("rollback CAS conflict", func(t *testing.T) {
		repo := newFakePolicyRepo()
		s := NewPolicyService(repo, &fakeGroupRepo{}, nil)
		if err := s.Create(context.Background(), policyRequestFixture()); err != nil {
			t.Fatalf("seed: %v", err)
		}
		// Two versions so rollback gets past "already at the oldest version".
		if err := s.Update(context.Background(), "test-policy", policyRequestFixture()); err != nil {
			t.Fatalf("seed v2: %v", err)
		}
		repo.forceConflict = true
		_, err := s.Rollback(context.Background(), "test-policy", "prev")
		if err == nil || err.Error() != wantModified {
			t.Fatalf("err = %v, want exactly %q", err, wantModified)
		}
		if !errors.Is(err, dbrepo.ErrConflict) {
			t.Fatalf("err = %v must still match dbrepo.ErrConflict", err)
		}
	})
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
	req.Name = nil
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
	resourceVersion := "999999999999"
	req.ResourceVersion = &resourceVersion
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
