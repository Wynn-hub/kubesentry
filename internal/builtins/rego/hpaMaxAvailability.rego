package kubesentry

deny[msg] {
  min := input.request.object.spec.minReplicas
  max := input.request.object.spec.maxReplicas
  max <= min
  msg := sprintf("HPA maxReplicas (%d) must be greater than minReplicas (%d)", [max, min])
}
