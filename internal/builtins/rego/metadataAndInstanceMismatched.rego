package kubesentry

_instance_label(obj) = val {
  val := obj.metadata.labels["app.kubernetes.io/instance"]
}

deny[msg] {
  name := input.request.object.metadata.name
  instance := _instance_label(input.request.object)
  name != instance
  msg := sprintf("metadata.name %q does not match app.kubernetes.io/instance label %q", [name, instance])
}
