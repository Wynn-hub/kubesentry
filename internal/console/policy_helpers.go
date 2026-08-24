package console

const (
	cursorAnnotation = "kubesentry.io/logical-cursor"
	builtinLabel     = "kubesentry.io/builtin"
)

func policySource(labels map[string]string) string {
	if labels[builtinLabel] == "true" {
		return "builtin"
	}
	return "custom"
}

// Keep this helper next to the remaining legacy/shared console code. The
// group handlers still use the same builtin label convention as policy DTOs.
