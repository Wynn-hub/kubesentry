package kubesentry

_containers[c] {
  input.request.resource.resource == "pods"
  c := input.request.object.spec.containers[_]
}
_containers[c] {
  input.request.resource.resource != "pods"
  c := input.request.object.spec.template.spec.containers[_]
}

_has_seccomp(c) {
  c.securityContext.seccompProfile.type
}
_has_selinux(c) {
  c.securityContext.seLinuxOptions.type
}
_has_drop_caps(c) {
  c.securityContext.capabilities.drop[_]
}

deny[msg] {
  c := _containers[_]
  not _has_seccomp(c)
  not _has_selinux(c)
  not _has_drop_caps(c)
  msg := sprintf("container %q has no Linux hardening (set seccompProfile, seLinuxOptions, or capabilities.drop)", [c.name])
}
