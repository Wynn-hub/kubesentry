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
  port := c.ports[_]
  port.hostPort > 0
  msg := sprintf("container %q must not set hostPort %d", [c.name, port.hostPort])
}
