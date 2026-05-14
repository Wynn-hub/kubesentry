#!/bin/bash

# 为所有 37 条内置规则生成 Policy 对象

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POLICIES_DIR="$SCRIPT_DIR/policies"

mkdir -p "$POLICIES_DIR"

# 从 builtins library 中提取所有规则并创建 Policy 对象
# 这是一个简化的示例 - 实际会直接从 Go 代码中读取

cat > "$POLICIES_DIR/all-policies.yaml" << 'EOF'
---
# 安全规则
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: privilege-escalation-allowed
  namespace: kubesentry-system
spec:
  enforcementMode: enforce
  rego: |
    package kubesentry
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
      c.securityContext.allowPrivilegeEscalation == true
      msg := sprintf("container %q must not allow privilege escalation", [c.name])
    }
  match:
    operations: ["CREATE", "UPDATE"]
    resources:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
    - apiGroups: ["apps"]
      apiVersions: ["v1"]
      resources: ["deployments", "statefulsets", "daemonsets"]
---
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: host-pid-set
  namespace: kubesentry-system
spec:
  enforcementMode: enforce
  rego: |
    package kubesentry
    deny[msg] {
      input.request.object.spec.hostPID == true
      msg := "hostPID is not allowed"
    }
  match:
    operations: ["CREATE", "UPDATE"]
    resources:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
    - apiGroups: ["apps"]
      apiVersions: ["v1"]
      resources: ["deployments", "statefulsets", "daemonsets"]
---
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: tag-not-specified
  namespace: kubesentry-system
spec:
  enforcementMode: enforce
  rego: |
    package kubesentry
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
      image := c.image
      not contains(image, ":")
      msg := sprintf("container %q image must specify a tag", [c.name])
    }
    deny[msg] {
      c := _containers[_]
      image := c.image
      endswith(image, ":latest")
      msg := sprintf("container %q image tag cannot be latest", [c.name])
    }
  match:
    operations: ["CREATE", "UPDATE"]
    resources:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
    - apiGroups: ["apps"]
      apiVersions: ["v1"]
      resources: ["deployments", "statefulsets", "daemonsets"]
    - apiGroups: ["batch"]
      apiVersions: ["v1"]
      resources: ["jobs", "cronjobs"]
---
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: cpu-requests-missing
  namespace: kubesentry-system
spec:
  enforcementMode: audit
  rego: |
    package kubesentry
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
      not c.resources.requests.cpu
      msg := sprintf("container %q must set cpu requests", [c.name])
    }
  match:
    operations: ["CREATE", "UPDATE"]
    resources:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
    - apiGroups: ["apps"]
      apiVersions: ["v1"]
      resources: ["deployments", "statefulsets", "daemonsets"]
    - apiGroups: ["batch"]
      apiVersions: ["v1"]
      resources: ["jobs", "cronjobs"]
---
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: deployment-single-replica
  namespace: kubesentry-system
spec:
  enforcementMode: audit
  rego: |
    package kubesentry
    deny[msg] {
      input.request.resource.resource == "deployments"
      replicas := input.request.object.spec.replicas
      replicas == 1
      msg := "Deployment should have more than one replica"
    }
  match:
    operations: ["CREATE", "UPDATE"]
    resources:
    - apiGroups: ["apps"]
      apiVersions: ["v1"]
      resources: ["deployments"]
EOF

echo "✓ 生成了基础 Policy 对象"
echo "位置: $POLICIES_DIR/all-policies.yaml"
echo ""
echo "应用这些 Policy："
echo "  kubectl apply -f $POLICIES_DIR/all-policies.yaml"
