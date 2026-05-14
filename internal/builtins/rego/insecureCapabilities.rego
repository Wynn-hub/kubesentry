package kubesentry

_insecure := {
  "NET_BIND_SERVICE", "CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID",
  "KILL", "SETGID", "SETUID", "SETPCAP", "SETFCAP",
  "NET_BROADCAST", "IPC_LOCK", "IPC_OWNER",
  "SYS_BOOT", "SYS_NICE", "SYS_TIME", "SYS_TTY_CONFIG",
  "MKNOD", "AUDIT_WRITE", "AUDIT_CONTROL", "SYSLOG", "LINUX_IMMUTABLE",
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
  _insecure[cap]
  msg := sprintf("container %q adds insecure capability %q", [c.name, cap])
}
