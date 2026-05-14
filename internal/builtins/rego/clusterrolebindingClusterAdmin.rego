package kubesentry

deny[msg] {
  input.request.object.roleRef.name == "cluster-admin"
  msg := "ClusterRoleBinding references the cluster-admin ClusterRole"
}
