package kubesentry

deny[msg] {
  min := input.request.object.spec.minReplicas
  min <= 1
  msg := sprintf("HPA minReplicas (%d) should be greater than 1", [min])
}
