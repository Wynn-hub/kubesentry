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
  c.securityContext.allowPrivilegeEscalation == true
  msg := sprintf("container %q must not allow privilege escalation", [c.name])
}
deny[msg] {
  c := _init_containers[_]
  c.securityContext.allowPrivilegeEscalation == true
  msg := sprintf("initContainer %q must not allow privilege escalation", [c.name])
}
