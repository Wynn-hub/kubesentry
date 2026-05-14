package kubesentry

_exec_attach_verbs := {"create"}
_exec_attach_resources := {"pods/exec", "pods/attach"}

deny[msg] {
  rule := input.request.object.rules[_]
  resource := rule.resources[_]
  _exec_attach_resources[resource]
  verb := rule.verbs[_]
  _exec_attach_verbs[verb]
  msg := sprintf("ClusterRole grants %q on %q", [verb, resource])
}
