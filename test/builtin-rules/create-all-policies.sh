#!/bin/bash

# 为所有 37 条内置规则创建 Policy 对象
# 直接从源代码中提取 Rego 内容

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
POLICIES_DIR="$SCRIPT_DIR/policies"
REGO_DIR="/Users/weiying/go/src/Wynn-hub/kubesentry/internal/builtins/rego"

mkdir -p "$POLICIES_DIR"

echo "开始为所有 37 条规则创建 Policy 对象..."
echo ""

# 函数：为单个规则创建 Policy
create_policy() {
    local rule_name=$1
    local rego_file=$2
    local mode=$3  # enforce 或 audit
    local rule_key=$4

    if [ ! -f "$REGO_DIR/$rego_file" ]; then
        echo "✗ 跳过 $rule_name: Rego 文件不存在"
        return 1
    fi

    # 读取 Rego 内容
    local rego_content=$(cat "$REGO_DIR/$rego_file")

    cat >> "$POLICIES_DIR/all-policies.yaml" << POLICYEOF
---
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: $rule_key
  namespace: kubesentry-system
  labels:
    kubesentry.io/builtin: "true"
spec:
  enforcementMode: $mode
  rego: |
$(echo "$rego_content" | sed 's/^/    /')
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
    - apiGroups: ["rbac.authorization.k8s.io"]
      apiVersions: ["v1"]
      resources: ["roles", "rolebindings", "clusterroles", "clusterrolebindings"]
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["serviceaccounts"]
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["configmaps"]
    - apiGroups: ["networking.k8s.io"]
      apiVersions: ["v1"]
      resources: ["ingresses"]
    - apiGroups: ["autoscaling"]
      apiVersions: ["v2"]
      resources: ["horizontalpodautoscalers"]
POLICYEOF

    echo "✓ 创建 $rule_name"
}

# 清空文件
> "$POLICIES_DIR/all-policies.yaml"

# ==================== 安全规则 ====================
echo "== 安全规则 =="
create_policy "runAsPrivileged" "runAsPrivileged.rego" "enforce" "run-as-privileged"
create_policy "privilegeEscalationAllowed" "privilegeEscalationAllowed.rego" "enforce" "privilege-escalation-allowed"
create_policy "runAsRootAllowed" "runAsRootAllowed.rego" "audit" "run-as-root-allowed"
create_policy "notReadOnlyRootFilesystem" "notReadOnlyRootFilesystem.rego" "audit" "not-readonly-filesystem"
create_policy "linuxHardening" "linuxHardening.rego" "enforce" "linux-hardening"
create_policy "insecureCapabilities" "insecureCapabilities.rego" "audit" "insecure-capabilities"
create_policy "dangerousCapabilities" "dangerousCapabilities.rego" "enforce" "dangerous-capabilities"
create_policy "hostPIDSet" "hostPIDSet.rego" "enforce" "host-pid-set"
create_policy "hostIPCSet" "hostIPCSet.rego" "enforce" "host-ipc-set"
create_policy "hostNetworkSet" "hostNetworkSet.rego" "audit" "host-network-set"
create_policy "hostPortSet" "hostPortSet.rego" "audit" "host-port-set"
create_policy "sensitiveContainerEnvVar" "sensitiveContainerEnvVar.rego" "enforce" "sensitive-env-var"
create_policy "automountServiceAccountToken" "automountServiceAccountToken.rego" "audit" "automount-token"
create_policy "sensitiveConfigmapContent" "sensitiveConfigmapContent.rego" "enforce" "sensitive-configmap"
create_policy "tlsSettingsMissing" "tlsSettingsMissing.rego" "audit" "tls-settings-missing"
create_policy "clusterrolePodExecAttach" "clusterrolePodExecAttach.rego" "enforce" "clusterrole-exec"
create_policy "rolePodExecAttach" "rolePodExecAttach.rego" "enforce" "role-exec"
create_policy "clusterrolebindingPodExecAttach" "clusterrolebindingPodExecAttach.rego" "enforce" "clusterrolebinding-exec"
create_policy "rolebindingRolePodExecAttach" "rolebindingRolePodExecAttach.rego" "enforce" "rolebinding-role-exec"
create_policy "rolebindingClusterRolePodExecAttach" "rolebindingClusterRolePodExecAttach.rego" "enforce" "rolebinding-clusterrole-exec"
create_policy "clusterrolebindingClusterAdmin" "clusterrolebindingClusterAdmin.rego" "enforce" "clusterrolebinding-admin"
create_policy "rolebindingClusterAdminClusterRole" "rolebindingClusterAdminClusterRole.rego" "enforce" "rolebinding-admin-clusterrole"
create_policy "rolebindingClusterAdminRole" "rolebindingClusterAdminRole.rego" "enforce" "rolebinding-admin-role"

# ==================== 效率规则 ====================
echo ""
echo "== 效率规则 =="
create_policy "cpuRequestsMissing" "cpuRequestsMissing.rego" "audit" "cpu-requests-missing"
create_policy "memoryRequestsMissing" "memoryRequestsMissing.rego" "audit" "memory-requests-missing"
create_policy "cpuLimitsMissing" "cpuLimitsMissing.rego" "audit" "cpu-limits-missing"
create_policy "memoryLimitsMissing" "memoryLimitsMissing.rego" "audit" "memory-limits-missing"

# ==================== 可靠性规则 ====================
echo ""
echo "== 可靠性规则 =="
create_policy "readinessProbeMissing" "readinessProbeMissing.rego" "audit" "readiness-probe-missing"
create_policy "livenessProbeMissing" "livenessProbeMissing.rego" "audit" "liveness-probe-missing"
create_policy "tagNotSpecified" "tagNotSpecified.rego" "enforce" "tag-not-specified"
create_policy "pullPolicyNotAlways" "pullPolicyNotAlways.rego" "audit" "pull-policy-not-always"
create_policy "priorityClassNotSet" "priorityClassNotSet.rego" "audit" "priority-class-not-set"
create_policy "deploymentMissingReplicas" "deploymentMissingReplicas.rego" "audit" "deployment-missing-replicas"
create_policy "metadataAndInstanceMismatched" "metadataAndInstanceMismatched.rego" "audit" "metadata-instance-mismatch"
create_policy "topologySpreadConstraint" "topologySpreadConstraint.rego" "audit" "topology-spread-constraint"
create_policy "hpaMaxAvailability" "hpaMaxAvailability.rego" "audit" "hpa-max-availability"
create_policy "hpaMinAvailability" "hpaMinAvailability.rego" "audit" "hpa-min-availability"

echo ""
echo "✓ 完成！已为 37 条规则生成 Policy 对象"
echo ""
echo "应用所有 Policy："
echo "  kubectl apply -f $POLICIES_DIR/all-policies.yaml"
echo ""
echo "验证 Policy 创建："
echo "  kubectl get policy -n kubesentry-system | grep -c '^' "
