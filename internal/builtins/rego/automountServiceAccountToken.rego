package kubesentry

# Pods
deny[msg] {
  input.request.resource.resource == "pods"
  not input.request.object.spec.automountServiceAccountToken == false
  msg := "Pod should set spec.automountServiceAccountToken=false"
}

# ServiceAccounts
deny[msg] {
  input.request.resource.resource == "serviceaccounts"
  not input.request.object.automountServiceAccountToken == false
  msg := sprintf("ServiceAccount %q should set automountServiceAccountToken=false", [input.request.object.metadata.name])
}

# Workloads (deployments, statefulsets, daemonsets)
deny[msg] {
  r := input.request.resource.resource
  r != "pods"
  r != "serviceaccounts"
  not input.request.object.spec.template.spec.automountServiceAccountToken == false
  msg := sprintf("%s should set spec.template.spec.automountServiceAccountToken=false", [r])
}
