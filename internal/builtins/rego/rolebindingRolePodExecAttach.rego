package kubesentry

_known_exec_roles := {"pod-exec", "pod-attach", "exec-pod", "attach-pod"}

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "Role"
  role := _known_exec_roles[_]
  contains(lower(ref.name), role)
  msg := sprintf("RoleBinding references potentially dangerous Role %q", [ref.name])
}
