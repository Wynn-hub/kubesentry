package kubesentry

_sensitive_patterns := {
  "password", "passwd", "secret", "token", "key",
  "api_key", "apikey", "auth", "credential", "credentials",
  "private_key", "access_key", "private_token",
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
  env := c.env[_]
  name_lower := lower(env.name)
  pattern := _sensitive_patterns[_]
  contains(name_lower, pattern)
  msg := sprintf("container %q has potentially sensitive env var %q", [c.name, env.name])
}
