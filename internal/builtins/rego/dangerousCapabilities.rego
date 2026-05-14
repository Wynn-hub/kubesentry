package kubesentry

_dangerous := {
  "NET_ADMIN", "SYS_ADMIN", "SYS_PTRACE", "SYS_MODULE",
  "SYS_RAWIO", "DAC_READ_SEARCH", "NET_RAW", "SYS_RESOURCE",
  "SYS_PACCT", "SYS_NICE", "WAKE_ALARM",
}

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
  cap := c.securityContext.capabilities.add[_]
  _dangerous[cap]
  msg := sprintf("container %q adds dangerous capability %q", [c.name, cap])
}
