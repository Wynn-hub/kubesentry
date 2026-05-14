package kubesentry

_containers[c] {
  input.request.resource.resource == "pods"
  c := input.request.object.spec.containers[_]
}
_containers[c] {
  input.request.resource.resource != "pods"
  c := input.request.object.spec.template.spec.containers[_]
}
_init_containers[c] {
  input.request.resource.resource == "pods"
  c := input.request.object.spec.initContainers[_]
}
_init_containers[c] {
  input.request.resource.resource != "pods"
  c := input.request.object.spec.template.spec.initContainers[_]
}

deny[msg] {
  c := _containers[_]
  not c.securityContext.runAsNonRoot == true
  msg := sprintf("container %q should set securityContext.runAsNonRoot=true", [c.name])
}
deny[msg] {
  c := _init_containers[_]
  not c.securityContext.runAsNonRoot == true
  msg := sprintf("initContainer %q should set securityContext.runAsNonRoot=true", [c.name])
}
