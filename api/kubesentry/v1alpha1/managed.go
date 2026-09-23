package v1alpha1

// ManagedByLabel marks member-cluster objects materialized from a gateway
// distribution. Unlabelled objects remain member-owned.
const (
	ManagedByLabel    = "kubesentry.io/managed-by"
	ManagedByPlatform = "platform"
)

// LastTraceIDAnnotation records the trace that most recently changed a
// managed member-cluster object.
const LastTraceIDAnnotation = "kubesentry.io/last-trace-id"
