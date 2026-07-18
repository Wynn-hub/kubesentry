// Package console: RevalidateAllPolicies re-compiles every policy's Rego at
// process startup and refreshes compile_ok/compile_error — a defense
// against someone editing the database directly, bypassing the API's
// synchronous validation (spec D8).
package console

import (
	"context"
	"fmt"

	"github.com/Wynn-hub/kubesentry/internal/gateway"
	"github.com/Wynn-hub/kubesentry/internal/webhook"
)

func RevalidateAllPolicies(ctx context.Context, repos gateway.Repos) error {
	policies, err := repos.Policies.List(ctx)
	if err != nil {
		return fmt.Errorf("list policies for revalidation: %w", err)
	}
	for i := range policies {
		p := &policies[i]
		_, compileErr := webhook.CompileRego(p.Rego)
		p.CompileOK = compileErr == nil
		if compileErr != nil {
			p.CompileError = compileErr.Error()
		} else {
			p.CompileError = ""
		}
		if err := repos.Policies.Update(ctx, p); err != nil {
			return fmt.Errorf("update policy %q after revalidation: %w", p.Name, err)
		}
	}
	return nil
}
