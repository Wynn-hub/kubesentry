# KubeSentry Policy Examples

This directory contains sample policies and test resources for learning how to use KubeSentry.

## Prerequisites

- KubeSentry is [installed](../README.md#installation) on your Kubernetes cluster
- `kubectl` is configured to access your cluster
- (Optional) A local test cluster with [kind](https://kind.sigs.k8s.io/)

## Example Policies

### 1. Deny Privileged Containers

**File**: `policy-no-privileged.yaml`

**What it does**: Blocks Pods with `securityContext.privileged: true`.

**Resource coverage**: Pods only (not Deployments or other workload resources)

**Enforcement mode**: `enforce` (admissions are rejected)

**Usage:**

```bash
# Apply the policy
kubectl apply -f examples/policy-no-privileged.yaml

# Verify it was created
kubectl get policies
NAME                         PHASE   GENERATION   OBSERVEDGENERATION
deny-privileged-containers   Ready   1            1

# Check policy details
kubectl describe policy deny-privileged-containers
```

**Testing:**

```bash
# This Pod should be allowed (no privileged setting)
kubectl apply -f examples/test-normal-pod.yaml
# Result: Pod created successfully

# This Pod should be denied (privileged=true)
kubectl apply -f examples/test-privileged-pod.yaml
# Result: Error from server (Forbidden): admission webhook "validate.kubesentry.io" denied the request: [security/runAsPrivileged] ...

# Check the event for details
kubectl describe pod test-privileged
# Look for "Events" section with webhook rejection message
```

### 2. Deny Privileged Containers (with Deployments)

**File**: `policy-no-privileged-with-deployments.yaml`

**What it does**: Same as above, but also intercepts Deployments.

**Resource coverage**: Pods and Deployments

**Enforcement mode**: `enforce`

**Key difference**: Uses conditional logic in Rego to handle different container paths:
- Pod: `input.request.object.spec.containers[_]`
- Deployment: `input.request.object.spec.template.spec.containers[_]`

**Why two versions?**

- **Pod-only policy**: Covers all workloads (Deployments create Pods) but rejects happen after Pod creation
- **Pod + Deployment policy**: Rejects immediately when applying Deployment YAML; but requires separate Rego rules per resource type

Choose based on your feedback preference:
- Fail at `kubectl apply` time → use Pod + Deployment version
- Fail at Pod creation time → use Pod-only version (simpler)

**Usage:**

```bash
# Apply the policy
kubectl apply -f examples/policy-no-privileged-with-deployments.yaml

# Test with Deployment
kubectl apply -f examples/test-privileged-deployment.yaml
# Result: Rejected immediately during `kubectl apply`

# Test with normal Deployment
kubectl apply -f examples/test-normal-deployment.yaml
# Result: Deployment created successfully
```

## Testing Resources

This directory includes four test Kubernetes resources:

| File | Resource Type | Security Context | Expected Result |
|---|---|---|---|
| `test-normal-pod.yaml` | Pod | None | Allowed ✓ |
| `test-privileged-pod.yaml` | Pod | `privileged: true` | Denied ✗ |
| `test-normal-deployment.yaml` | Deployment (1 replica) | None | Allowed ✓ |
| `test-privileged-deployment.yaml` | Deployment (1 replica) | `privileged: true` | Denied ✗ |

## Writing Your Own Policies

### Step 1: Understand the Rego Contract

All Rego files must use the `kubesentry` package and expose a `deny` set:

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: my-policy
spec:
  enforcementMode: enforce  # or "audit"
  match:
    operations: [CREATE, UPDATE]
    resources:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: [pods]
  rego: |
    package kubesentry
    
    deny[msg] {
      # your condition
      msg := "user-friendly error message"
    }
```

### Step 2: Reference Request Data

The webhook passes the admission request as `input`:

```rego
input.request.object                        # The resource being admitted
input.request.operation                     # CREATE, UPDATE, DELETE, CONNECT
input.request.namespace                     # Target namespace
input.request.userInfo.username             # User performing the action
input.request.userInfo.groups[_]            # User's groups
```

### Step 3: Access Containers

Container paths depend on resource type:

| Resource | Path | Example |
|---|---|---|
| Pod | `.spec.containers[_]` | `input.request.object.spec.containers[_]` |
| Deployment | `.spec.template.spec.containers[_]` | `input.request.object.spec.template.spec.containers[_]` |
| DaemonSet | `.spec.template.spec.containers[_]` | |
| StatefulSet | `.spec.template.spec.containers[_]` | |
| Job | `.spec.template.spec.containers[_]` | |
| CronJob | `.spec.jobTemplate.spec.template.spec.containers[_]` | |

**Init containers** are under `.initContainers[_]` for all types.

### Step 4: Write Your Rego Logic

Example: require resource limits

```rego
package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.limits.memory
  msg := sprintf("container '%s' must have memory limit", [c.name])
}

deny[msg] {
  c := input.request.object.spec.containers[_]
  not c.resources.limits.cpu
  msg := sprintf("container '%s' must have CPU limit", [c.name])
}
```

Example: block certain image registries

```rego
package kubesentry

deny[msg] {
  c := input.request.object.spec.containers[_]
  image := c.image
  contains(image, "untrusted-registry.io")
  msg := sprintf("image '%s' from untrusted registry", [image])
}
```

### Step 5: Test with `audit` Mode First

When developing, use `audit` mode to test without blocking:

```yaml
spec:
  enforcementMode: audit  # Allows but logs warnings
```

Then switch to `enforce` after verification.

### Step 6: Deploy and Verify

```bash
# Create the policy
kubectl apply -f my-policy.yaml

# Wait for Ready
kubectl wait --for=condition=Ready policy/my-policy

# Test with your resources
kubectl apply -f test-pod.yaml

# Check warnings in response
kubectl apply -f test-pod.yaml 2>&1 | grep -i "warn"

# View policy logs
kubectl logs -n kubesentry-system -l app=webhook
```

## Enforcement Modes

### `enforce` Mode

**Behavior**: Admission requests matching violations are **rejected** (HTTP 403).

**User experience**: `kubectl apply` fails with violation message.

**Use for**: Security-critical policies (e.g., privileged containers, RBAC rules).

```yaml
spec:
  enforcementMode: enforce
```

### `audit` Mode

**Behavior**: Admission requests are **allowed**, violations logged as warnings.

**User experience**: `kubectl apply` succeeds but shows warning.

**Use for**: Best-practices policies (e.g., missing liveness probe, no resource limits) that you want to enforce gradually.

```yaml
spec:
  enforcementMode: audit
```

To see audit warnings:

```bash
kubectl apply -f my-pod.yaml 2>&1
# Output includes:
# Warning: [efficiency/cpuRequestsMissing] container "app" does not have CPU requests set
```

## Matching Resources Precisely

The `match` field lets you control which resources trigger the policy:

```yaml
spec:
  match:
    # Which Kubernetes operations trigger this policy
    operations: [CREATE, UPDATE]  # or [DELETE, CONNECT]
    
    # Which resource types to match
    resources:
      # Core API Group (empty string)
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
      
      # Apps API Group (Deployments, etc.)
      - apiGroups: ["apps"]
        apiVersions: ["v1"]
        resources: ["deployments", "statefulsets", "daemonsets"]
      
      # Namespace selector (optional)
      namespaceSelector:
        matchExpressions:
          - key: environment
            operator: In
            values: [production]
```

## Violation Message Format

When a policy denies an admission:

```
[<group>/<key>] <message>
  描述：<description>
```

Example:

```
[security/runAsPrivileged] container "app" cannot run as privileged
  描述：Blocks containers with securityContext.privileged == true
```

The group, key, and description come from the Policy's labels and status set by the operator.

## Using PolicyGroup for Organization

For more complex scenarios with multiple related policies, use `PolicyGroup`:

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: PolicyGroup
metadata:
  name: my-security-group
spec:
  enabled: true
  displayName: "My Security Policies"
  description: "Custom security policies for our team"
  policies:
    # Include a built-in policy with mode override
    - key: runAsPrivileged
      mode: enforce

    # Add a custom policy
    - key: noDebugContainers
      mode: enforce
      rego: |
        package kubesentry
        deny[msg] {
          c := input.request.object.spec.containers[_]
          c.name == "debug"
          msg := "debug containers are not allowed"
        }
      match:
        operations: [CREATE, UPDATE]
        resources:
          - apiGroups: [""]
            apiVersions: ["v1"]
            resources: [pods]
```

See [PolicyGroup in README.md](../README.md#policygroup-crd) for details.

## Troubleshooting

### Policy Not Matching Resources

```bash
# Check if policy is in Ready state
kubectl get policy my-policy
# Should show Phase: Ready

# If not Ready, check status
kubectl describe policy my-policy
# Look for Conditions section

# Check webhook logs
kubectl logs -n kubesentry-system -l app=webhook | grep -i "my-policy"
```

### Webhook Rejecting but No Policy Created

```bash
# Verify policy exists
kubectl get policies
kubectl describe policy <name>

# Check webhook logs for errors
kubectl logs -n kubesentry-system -l app=webhook -f

# Verify webhook is running
kubectl get pods -n kubesentry-system
```

### Rego Compilation Errors

When deploying a Policy with invalid Rego:

```bash
kubectl apply -f bad-policy.yaml

# Check the error message
kubectl describe policy bad-policy
# Status.Message will show the compilation error
```

Common Rego errors:
- `package kubesentry` missing — add this line at the top
- `deny[msg]` syntax — must use `deny` set (not `allow`)
- Undefined variables — use `_` for wildcards
- v1 syntax (e.g., `some x in set`) — use v0 syntax with `set[_]` instead

### Understanding Input Structure

To debug what data is available in `input`:

```rego
package kubesentry

deny[msg] {
  # Inspect the full input
  msg := sprintf("Input: %v", [input])
}
```

Then check the webhook logs to see the actual request structure.

## Performance Considerations

- **Policies evaluated in parallel** — Multiple policies run concurrently per request
- **5-second timeout** — Slow Rego rules are killed after 5 seconds
- **Cache-based** — Policies are pre-compiled and cached; compile errors caught at Policy creation time (not per request)

Avoid expensive operations in Rego:
- Don't fetch Kubernetes resources (static Rego only)
- Keep string operations simple
- Use efficient set matching

## Further Reading

- [OPA Rego Policy Language](https://www.openpolicyagent.org/docs/latest/policy-language/)
- [Kubernetes Admission Webhooks](https://kubernetes.io/docs/reference/access-authn-authz/webhook/)
- [KubeSentry README](../README.md) — Full documentation
- [CONTRIBUTING.md](../CONTRIBUTING.md) — How to contribute new examples or policies
