package kubesentry

deny[msg] {
  ref := input.request.object.roleRef
  ref.kind == "ClusterRole"
  ref.name == "cluster-admin"
  msg := "RoleBinding references the cluster-admin ClusterRole"
}
