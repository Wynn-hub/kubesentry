package kubesentry

_containers[c] {
  input.request.resource.resource == "pods"
  c := input.request.object.spec.containers[_]
}
_containers[c] {
  r := input.request.resource.resource
  r != "pods"
  r != "cronjobs"
  c := input.request.object.spec.template.spec.containers[_]
}
_containers[c] {
  input.request.resource.resource == "cronjobs"
  c := input.request.object.spec.jobTemplate.spec.template.spec.containers[_]
}

deny[msg] {
  c := _containers[_]
  c.imagePullPolicy != "Always"
  msg := sprintf("container %q imagePullPolicy should be Always, got %q", [c.name, c.imagePullPolicy])
}
