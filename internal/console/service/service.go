// Package service holds the console's business logic. It is the only layer
// that holds a repository: handlers must go through it, so no HTTP handler can
// reach the database directly.
package service

import "github.com/Wynn-hub/kubesentry/internal/console/repository"

// Services is the bundle handlers receive. It grows one field per migrated
// resource domain.
type Services struct {
	Policy *PolicyService
}

// New wires every service from the repository bundle.
func New(repos repository.Repositories) Services {
	return Services{
		Policy: NewPolicyService(repos.Policies),
	}
}
