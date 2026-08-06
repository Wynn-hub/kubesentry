package repository

import "github.com/Wynn-hub/kubesentry/internal/gateway"

// Repositories is the console's data-access bundle. It grows one field per
// migrated resource domain; batches that haven't migrated yet keep using
// gateway.Repos directly from the legacy handlers.
type Repositories struct {
	Policies PolicyRepository
	Groups   GroupRepository
}

// FromGatewayRepos narrows the shared gateway.Repos bundle down to the
// console's interfaces. cmd/console keeps constructing gateway.Repos (it is
// what internal/gateway and cmd/db also use); this is the single adaptation
// point rather than threading concrete repo types through the service layer.
func FromGatewayRepos(r gateway.Repos) Repositories {
	return Repositories{
		Policies: r.Policies,
		Groups:   r.Groups,
	}
}
