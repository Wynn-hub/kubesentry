# KubeSentry Platform Chart

Installs Gateway, Console Web/API, PostgreSQL, and the database migration job.

```bash
helm install kubesentry-gateway . -n kubesentry-system --create-namespace
```

Default external services:

| Service | Port | Purpose |
|---|---:|---|
| Console NodePort | TCP 30080 (Service 8080) | Administrator UI |
| Gateway NodePort | TLS/TCP 30443 (Service 9443) | Member-cluster connections |

PostgreSQL 5432 and Console API 8081 should remain internal. For production, use HTTPS for Console, set `console.session.cookieSecure=true`, use managed credentials or `postgres.external.dsn`, and restrict Gateway to member-cluster source addresses. See [`values.yaml`](values.yaml), the [installation guide](../../docs/en-US/installation.md), and [network guide](../../docs/en-US/network.md).
