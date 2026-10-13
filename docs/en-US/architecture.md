# Architecture and components

KubeSentry has platform and member-cluster planes. Console Web/API, Gateway, PostgreSQL, and the migration job store identities, tenants, policy intent, and events. Operator, Webhook, Events Server, and CRDs enforce admission in each member cluster.

![Deployment architecture](../diagrams/deployment-architecture.svg)

![Component flows](../diagrams/component-architecture.svg)

Gateway sends desired state over a TLS gRPC stream initiated by Operator; status, heartbeat, events, and observations travel upward. The API server calls Webhook through Service 443. Webhook reads its compiled informer cache and never synchronously queries the platform or database on the admission path.

System administrators have no tenant; tenant administrators and viewers belong to exactly one. Server middleware and services are the authorization boundary.
