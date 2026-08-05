// Package model aliases the database entities so the dto conversion functions
// read as model.Policy -> dto.PolicyListItem, making the layer boundary
// explicit in each signature.
//
// These are aliases, not copies: internal/dbrepo is shared with cmd/gateway
// and must not be moved or duplicated. The alias gives the console's five-layer
// structure its model layer without forking the entity definitions.
package model

import "github.com/Wynn-hub/kubesentry/internal/dbrepo"

type (
	Policy        = dbrepo.Policy
	PolicyVersion = dbrepo.PolicyVersion
)
