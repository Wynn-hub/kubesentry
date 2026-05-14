#!/bin/bash

# 脚本用途：验证所有内置规则是否正确工作
# 用法: bash test-builtin-rules.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_NS="test-builtin-rules"
KUBESENTRY_NS="kubesentry-system"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ==================== 辅助函数 ====================

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_kubesentry() {
    log_info "检查 kubesentry 是否已部署..."
    if ! kubectl get namespace $KUBESENTRY_NS &> /dev/null; then
        log_error "kubesentry-system 命名空间不存在"
        exit 1
    fi

    if ! kubectl get deployment -n $KUBESENTRY_NS | grep -q webhook; then
        log_error "webhook deployment 未找到"
        exit 1
    fi

    if ! kubectl get deployment -n $KUBESENTRY_NS | grep -q operator; then
        log_error "operator deployment 未找到"
        exit 1
    fi

    log_info "✓ kubesentry 已正确部署"
}

setup_test_namespace() {
    log_info "设置测试命名空间..."
    if kubectl get namespace $TEST_NS &> /dev/null; then
        log_warn "命名空间 $TEST_NS 已存在，清理旧资源..."
        kubectl delete namespace $TEST_NS --ignore-not-found=true
        sleep 3
    fi

    kubectl create namespace $TEST_NS
    log_info "✓ 命名空间 $TEST_NS 已创建"
}

enable_policies() {
    log_info "启用内置规则..."

    # 检查 enable-builtin-policies.yaml 是否存在
    if [ ! -f "$SCRIPT_DIR/enable-builtin-policies.yaml" ]; then
        log_error "enable-builtin-policies.yaml 未找到"
        exit 1
    fi

    kubectl apply -f "$SCRIPT_DIR/enable-builtin-policies.yaml"

    # 等待 PolicyGroup 被operator处理
    log_info "等待 PolicyGroup 被处理..."
    sleep 5

    # 验证 Policy 对象是否已创建
    policy_count=$(kubectl get policy -n $KUBESENTRY_NS --no-headers 2>/dev/null | wc -l)
    if [ "$policy_count" -lt 30 ]; then
        log_warn "Expected at least 30 policies, got $policy_count. Wait a bit more..."
        sleep 5
    fi

    policy_count=$(kubectl get policy -n $KUBESENTRY_NS --no-headers 2>/dev/null | wc -l)
    log_info "✓ 已创建 $policy_count 条 Policy"
}

run_test() {
    local test_file=$1
    local rule_name=$2
    local should_fail=$3  # true or false

    local test_name=$(basename "$test_file" .yaml)
    local test_type=$(basename $(dirname "$test_file"))

    echo -n "  测试 $rule_name ($test_name)... "

    if kubectl apply -f "$test_file" 2>&1 | grep -q "admission webhook"; then
        # 被 webhook 拒绝
        if [ "$should_fail" = "true" ]; then
            echo -e "${GREEN}✓ 正确被拒绝${NC}"
            return 0
        else
            echo -e "${RED}✗ 不应该被拒绝${NC}"
            return 1
        fi
    else
        # 被 webhook 接受
        if [ "$should_fail" = "false" ]; then
            echo -e "${GREEN}✓ 正确被接受${NC}"
            # 清理
            kubectl delete -f "$test_file" --ignore-not-found=true 2>/dev/null || true
            return 0
        else
            echo -e "${RED}✗ 不应该被接受${NC}"
            # 清理
            kubectl delete -f "$test_file" --ignore-not-found=true 2>/dev/null || true
            return 1
        fi
    fi
}

test_security_rules() {
    log_info "=== 测试安全规则 ==="

    local passed=0
    local failed=0

    # runAsPrivileged - 应该失败
    if run_test "$SCRIPT_DIR/security/run-as-privileged-fail.yaml" "runAsPrivileged" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # privilegeEscalationAllowed - 应该失败
    if run_test "$SCRIPT_DIR/security/privilege-escalation-fail.yaml" "privilegeEscalationAllowed" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # runAsRootAllowed - 应该失败（audit模式下会产生warning）
    if run_test "$SCRIPT_DIR/security/run-as-root-fail.yaml" "runAsRootAllowed" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # notReadOnlyRootFilesystem - 应该失败
    if run_test "$SCRIPT_DIR/security/not-readonly-filesystem-fail.yaml" "notReadOnlyRootFilesystem" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # linuxHardening - 应该失败
    if run_test "$SCRIPT_DIR/security/linux-hardening-fail.yaml" "linuxHardening" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # insecureCapabilities - 应该失败
    if run_test "$SCRIPT_DIR/security/insecure-capabilities-fail.yaml" "insecureCapabilities" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # dangerousCapabilities - 应该失败
    if run_test "$SCRIPT_DIR/security/dangerous-capabilities-fail.yaml" "dangerousCapabilities" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # hostPIDSet - 应该失败
    if run_test "$SCRIPT_DIR/security/host-pid-fail.yaml" "hostPIDSet" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # hostIPCSet - 应该失败
    if run_test "$SCRIPT_DIR/security/host-ipc-fail.yaml" "hostIPCSet" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # hostNetworkSet - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/security/host-network-fail.yaml" "hostNetworkSet" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # hostPortSet - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/security/host-port-fail.yaml" "hostPortSet" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # sensitiveContainerEnvVar - 应该失败
    if run_test "$SCRIPT_DIR/security/sensitive-env-var-fail.yaml" "sensitiveContainerEnvVar" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # automountServiceAccountToken - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/security/automount-token-fail.yaml" "automountServiceAccountToken" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # sensitiveConfigmapContent - 应该失败
    if run_test "$SCRIPT_DIR/security/sensitive-configmap-fail.yaml" "sensitiveConfigmapContent" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # tlsSettingsMissing - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/security/tls-settings-missing-fail.yaml" "tlsSettingsMissing" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    echo ""
    echo -e "安全规则测试: ${GREEN}$passed passed${NC}, ${RED}$failed failed${NC}"

    return $failed
}

test_efficiency_rules() {
    log_info "=== 测试效率规则 ==="

    local passed=0
    local failed=0

    # cpuRequestsMissing - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/efficiency/cpu-requests-missing-fail.yaml" "cpuRequestsMissing" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # memoryRequestsMissing - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/efficiency/memory-requests-missing-fail.yaml" "memoryRequestsMissing" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # cpuLimitsMissing - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/efficiency/cpu-limits-missing-fail.yaml" "cpuLimitsMissing" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # memoryLimitsMissing - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/efficiency/memory-limits-missing-fail.yaml" "memoryLimitsMissing" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    echo ""
    echo -e "效率规则测试: ${GREEN}$passed passed${NC}, ${RED}$failed failed${NC}"

    return $failed
}

test_reliability_rules() {
    log_info "=== 测试可靠性规则 ==="

    local passed=0
    local failed=0

    # readinessProbeMissing - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/readiness-probe-missing-fail.yaml" "readinessProbeMissing" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # livenessProbeMissing - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/liveness-probe-missing-fail.yaml" "livenessProbeMissing" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # tagNotSpecified - 应该失败 (latest)
    if run_test "$SCRIPT_DIR/reliability/tag-not-specified-fail.yaml" "tagNotSpecified" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # tagNotSpecified - 应该失败 (no tag)
    if run_test "$SCRIPT_DIR/reliability/tag-not-specified-fail-2.yaml" "tagNotSpecified" "true"; then
        ((passed++))
    else
        ((failed++))
    fi

    # pullPolicyNotAlways - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/pull-policy-not-always-fail.yaml" "pullPolicyNotAlways" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # priorityClassNotSet - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/priority-class-not-set-fail.yaml" "priorityClassNotSet" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # deploymentMissingReplicas - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/deployment-single-replica-fail.yaml" "deploymentMissingReplicas" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # metadataAndInstanceMismatched - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/metadata-instance-mismatch-fail.yaml" "metadataAndInstanceMismatched" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # topologySpreadConstraint - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/topology-spread-missing-fail.yaml" "topologySpreadConstraint" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # hpaMaxAvailability - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/hpa-max-availability-fail.yaml" "hpaMaxAvailability" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    # hpaMinAvailability - 应该失败（audit模式）
    if run_test "$SCRIPT_DIR/reliability/hpa-min-availability-fail.yaml" "hpaMinAvailability" "false"; then
        ((passed++))
    else
        ((failed++))
    fi

    echo ""
    echo -e "可靠性规则测试: ${GREEN}$passed passed${NC}, ${RED}$failed failed${NC}"

    return $failed
}

cleanup() {
    log_info "清理测试资源..."
    kubectl delete namespace $TEST_NS --ignore-not-found=true 2>/dev/null || true
    log_info "✓ 测试资源已清理"
}

# ==================== 主程序 ====================

main() {
    echo "======================================"
    echo "KubeSentry 内置规则测试"
    echo "======================================"
    echo ""

    check_kubesentry
    setup_test_namespace
    enable_policies

    echo ""

    total_failed=0

    test_security_rules
    ((total_failed+=$?))

    echo ""

    test_efficiency_rules
    ((total_failed+=$?))

    echo ""

    test_reliability_rules
    ((total_failed+=$?))

    echo ""
    echo "======================================"

    cleanup

    if [ $total_failed -eq 0 ]; then
        log_info "所有测试通过！"
        exit 0
    else
        log_error "有 $total_failed 个测试失败"
        exit 1
    fi
}

main "$@"
