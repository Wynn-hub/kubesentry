package kubesentry

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "Role"
  lower(ref.name) == "cluster-admin"
  msg := sprintf("RoleBinding references a Role named %q which may grant cluster-admin permissions", [ref.name])
}
