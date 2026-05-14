package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("RoleBinding references potentially dangerous ClusterRole %q", [ref.name])
}
