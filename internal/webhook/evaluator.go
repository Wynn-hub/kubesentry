package webhook

import (
	"context"
	"fmt"

	"github.com/open-policy-agent/opa/rego"
)

// CompileRego pre-compiles a Rego policy string for repeated evaluation.
// The policy must use package kubesentry and define deny[msg].
func CompileRego(regoContent string) (rego.PreparedEvalQuery, error) {
	r := rego.New(
		rego.Query("data.kubesentry.deny"),
		rego.Module("policy.rego", regoContent),
	)
	q, err := r.PrepareForEval(context.Background())
	if err != nil {
		return rego.PreparedEvalQuery{}, fmt.Errorf("compile rego: %w", err)
	}
	return q, nil
}

// EvaluatePolicy runs a pre-compiled query and returns the deny messages.
// An empty slice means the policy allows the request.
func EvaluatePolicy(ctx context.Context, q rego.PreparedEvalQuery, input map[string]interface{}) ([]string, error) {
	rs, err := q.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return nil, fmt.Errorf("opa eval: %w", err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return nil, nil
	}
	raw, ok := rs[0].Expressions[0].Value.([]interface{})
	if !ok || len(raw) == 0 {
		return nil, nil
	}
	msgs := make([]string, 0, len(raw))
	for _, v := range raw {
		if msg, ok := v.(string); ok {
			msgs = append(msgs, msg)
		}
	}
	return msgs, nil
}
