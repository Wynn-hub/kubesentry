package kubesentry

_spec(obj) = obj.spec {
  input.request.resource.resource == "pods"
}
_spec(obj) = obj.spec.template.spec {
  input.request.resource.resource != "pods"
}

deny[msg] {
  spec := _spec(input.request.object)
  not spec.topologySpreadConstraints
  msg := "no topologySpreadConstraints defined"
}
deny[msg] {
  spec := _spec(input.request.object)
  count(spec.topologySpreadConstraints) == 0
  msg := "topologySpreadConstraints is empty"
}
