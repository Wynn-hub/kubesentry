package kubesentry

deny[msg] {
  count(input.request.object.spec.tls) == 0
  msg := "Ingress should configure TLS settings"
}

deny[msg] {
  not input.request.object.spec.tls
  msg := "Ingress should configure TLS settings"
}
