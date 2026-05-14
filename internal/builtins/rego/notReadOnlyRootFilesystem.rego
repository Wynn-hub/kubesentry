package kubesentry

_containers[c] {
  input.request.resource.resource == "pods"
  c := input.request.object.spec.containers[_]
}
_containers[c] {
  input.request.resource.resource != "pods"
  c := input.request.object.spec.template.spec.containers[_]
}

deny[msg] {
  c := _containers[_]
  not c.securityContext.readOnlyRootFilesystem == true
  msg := sprintf("container %q should set securityContext.readOnlyRootFilesystem=true", [c.name])
}
