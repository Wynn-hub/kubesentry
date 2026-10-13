# Network policy and ports

| Source | Destination | Port | Direction / notes |
|---|---|---:|---|
| Browser | Console NodePort/Ingress | TCP 30080/custom | Inbound; use HTTPS in production |
| Console Web | Console API | TCP 8081 | Same-Pod localhost |
| Console API | Gateway | TCP 9443 | Platform cluster internal |
| Operator | Gateway | TLS/TCP 30443 | Member-initiated egress; Service 9443 |
| Console API, Gateway, DB Job | PostgreSQL | TCP 5432 | Platform cluster internal |
| kube-apiserver | Webhook Service | TCP 443 | Member internal; target 8443 |
| Webhook | Operator Events | TCP 8082 | Member internal |
| Components | Kubernetes API / DNS | TCP 443 / UDP,TCP 53 | Cluster infrastructure |
| Nodes | Image registry | TCP 443 | Egress for image pulls |

NetworkPolicy should allow only the stated source Pod selectors and deny unrelated namespaces direct access to PostgreSQL, Console API, Events Server, and Webhook container ports. With NodePort, open Console 30080 only to administrator sources and Gateway 30443 to member-cluster egress addresses. Any values override must update this guide, both READMEs, Help Center, and firewall rules.
