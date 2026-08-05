package service

import "github.com/Wynn-hub/kubesentry/internal/console/repository"

// PolicyService owns the policy domain's business logic. Methods are added in
// Task 4 as the legacy handlers_policy.go logic moves here.
type PolicyService struct {
	repo repository.PolicyRepository
}

func NewPolicyService(repo repository.PolicyRepository) *PolicyService {
	return &PolicyService{repo: repo}
}
