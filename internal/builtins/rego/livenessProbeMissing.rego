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
  not c.livenessProbe
  msg := sprintf("container %q must define a livenessProbe", [c.name])
}
