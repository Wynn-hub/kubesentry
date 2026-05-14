package kubesentry

_sensitive_keys := {
  "password", "passwd", "secret", "token", "api_key", "apikey",
  "private_key", "access_key", "credentials", "credential",
}

deny[msg] {
  key := input.request.object.data[k]
  k_lower := lower(k)
  pattern := _sensitive_keys[_]
  contains(k_lower, pattern)
  msg := sprintf("ConfigMap key %q may contain sensitive data", [k])
}
