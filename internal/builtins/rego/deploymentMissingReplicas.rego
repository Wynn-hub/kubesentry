package kubesentry

deny[msg] {
  replicas := input.request.object.spec.replicas
  replicas < 2
  msg := sprintf("Deployment has only %d replica(s); set replicas >= 2 for availability", [replicas])
}
deny[msg] {
  not input.request.object.spec.replicas
  msg := "Deployment does not specify replicas; default of 1 is insufficient for availability"
}
