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
  image := c.image
  not contains(image, ":")
  msg := sprintf("container %q image %q has no tag", [c.name, image])
}
deny[msg] {
  c := _containers[_]
  image := c.image
  endswith(image, ":latest")
  msg := sprintf("container %q image %q uses the latest tag", [c.name, image])
}
