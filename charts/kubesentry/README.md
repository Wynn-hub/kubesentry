# KubeSentry Helm Chart

Deploys KubeSentry — a Kubernetes Validating Admission Webhook with an OPA/Rego policy engine and CRD-based policy management.

## Components

| Component | Description |
|---|---|
| **Webhook** | Serves `/validate` admission requests; evaluates Rego policies from an in-memory cache |
| **Operator** | Reconciles `Policy` CRDs; manages `PolicyVersion` snapshots, rollback, and `ValidatingWebhookConfiguration` rules |
| **TLS Setup Job** | Pre-install Hook that generates a self-signed CA + server cert, writes a TLS Secret, and patches `caBundle` |

## Prerequisites

- Kubernetes 1.28+
- Helm 3.8+

## Installation

```bash
# From Docker Hub (OCI)
helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --version 0.1.0 \
  --namespace kubesentry-system \
  --create-namespace

# From local chart
helm install kubesentry . \
  --namespace kubesentry-system \
  --create-namespace
```

## Upgrade

```bash
helm upgrade kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --version 0.2.0 \
  --namespace kubesentry-system
```

## Uninstall

```bash
helm uninstall kubesentry -n kubesentry-system
kubectl delete namespace kubesentry-system
# CRDs are not removed automatically
kubectl delete crd policies.kubesentry.io policyversions.kubesentry.io
```

## Values

| Key | Default | Description |
|---|---|---|
| `webhook.replicas` | `2` | Webhook Deployment replicas |
| `webhook.image.repository` | `wynnhub/kubesentry-webhook` | Webhook image repository |
| `webhook.image.tag` | `latest` | Webhook image tag |
| `webhook.image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `webhook.resources.requests.cpu` | `100m` | CPU request |
| `webhook.resources.requests.memory` | `128Mi` | Memory request |
| `webhook.resources.limits.cpu` | `500m` | CPU limit |
| `webhook.resources.limits.memory` | `256Mi` | Memory limit |
| `operator.replicas` | `1` | Operator Deployment replicas |
| `operator.image.repository` | `wynnhub/kubesentry-operator` | Operator image repository |
| `operator.image.tag` | `latest` | Operator image tag |
| `operator.image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `operator.resources.requests.cpu` | `50m` | CPU request |
| `operator.resources.requests.memory` | `64Mi` | Memory request |
| `tls.secretName` | `kubesentry-tls` | Name of the TLS Secret created by the setup Job |
| `failurePolicy` | `Fail` | `ValidatingWebhookConfiguration` failure policy (`Fail` or `Ignore`) |
| `policy.versionHistoryLimit` | `20` | Maximum number of `PolicyVersion` objects retained per Policy |
| `webhookNamespaceSelector` | excludes `kube-system`, `kubesentry-system` | Namespace selector applied to the webhook |

## TLS

The pre-install Job runs `kubesentry-operator tls-setup`, which:

1. Generates an ECDSA P-256 self-signed CA and server certificate (valid 365 days)
2. Stores them in a Secret (`tls.secretName`) with keys `ca.crt`, `tls.crt`, `tls.key`
3. Patches the `ValidatingWebhookConfiguration` `caBundle` field

The Job is idempotent — if the Secret already exists it exits without making changes.

## Policy CRD quick reference

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: example
spec:
  enforcementMode: enforce   # enforce | audit
  match:
    operations: [CREATE, UPDATE]
    resources:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
  rego: |
    package kubesentry
    deny[msg] {
      # your rule here
      msg := "reason"
    }
```

To roll back to a previous version:

```yaml
spec:
  rollbackTo:
    version: 3
```

## Source

- GitHub: https://github.com/Wynn-hub/kubesentry
- Images: https://hub.docker.com/u/wynnhub
