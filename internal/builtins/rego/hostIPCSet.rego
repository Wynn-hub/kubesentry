package kubesentry

_spec(obj) = obj.spec {
  input.request.resource.resource == "pods"
}
_spec(obj) = obj.spec.template.spec {
  input.request.resource.resource != "pods"
}

deny[msg] {
  spec := _spec(input.request.object)
  spec.hostIPC == true
  msg := "hostIPC must not be set"
}
