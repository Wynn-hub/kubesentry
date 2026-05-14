#!/bin/bash

# 完整验证脚本 - 测试所有 37 条规则

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_NS="test-builtin-rules"
KUBESENTRY_NS="kubesentry-system"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 测试结果统计
declare -i total=0
declare -i passed=0
declare -i failed=0
declare -a results=()

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_pass() { echo -e "${GREEN}✓${NC} $1"; }
log_fail() { echo -e "${RED}✗${NC} $1"; }
log_test() { echo -e "${BLUE}[TEST]${NC} $1"; }

# 测试单个规则
test_rule() {
    local rule_name=$1
    local yaml_file=$2
    local expected_behavior=$3  # "deny" 或 "allow"

    ((total++))

    if [ ! -f "$SCRIPT_DIR/$yaml_file" ]; then
        log_fail "$rule_name (文件不存在: $yaml_file)"
        ((failed++))
        results+=("SKIP|$rule_name|文件不存在")
        return
    fi

    # 应用测试资源
    local output=$(kubectl apply -f "$SCRIPT_DIR/$yaml_file" -n $TEST_NS 2>&1)

    if [ "$expected_behavior" = "deny" ]; then
        # 期望被拒绝
        if echo "$output" | grep -qE "denied|Forbidden"; then
            log_pass "$rule_name (被正确拒绝)"
            ((passed++))
            results+=("PASS|$rule_name|规则正确触发，资源被拒绝")
        else
            log_fail "$rule_name (应该被拒绝但被接受)"
            ((failed++))
            results+=("FAIL|$rule_name|规则未触发")
            kubectl delete -f "$SCRIPT_DIR/$yaml_file" -n $TEST_NS 2>/dev/null || true
        fi
    else
        # 期望被接受
        if echo "$output" | grep -qE "created|configured"; then
            log_pass "$rule_name (被正确接受 - audit 模式)"
            ((passed++))
            results+=("PASS|$rule_name|审计模式：资源被接受（需检查日志）")
            kubectl delete -f "$SCRIPT_DIR/$yaml_file" -n $TEST_NS 2>/dev/null || true
        else
            log_fail "$rule_name (应该被接受但被拒绝)"
            ((failed++))
            results+=("FAIL|$rule_name|不应该被拒绝")
        fi
    fi
}

# 显示 webhook 日志
show_webhook_logs() {
    echo ""
    echo -e "${BLUE}=== Webhook 日志（最后 10 条）===${NC}"
    kubectl logs -n $KUBESENTRY_NS deployment/kubesentry-webhook --tail=10 2>/dev/null || echo "无法获取日志"
}

# ==================== 主程序 ====================

main() {
    echo "=========================================="
    echo "KubeSentry 完整规则验证测试"
    echo "=========================================="
    echo ""

    # 清理旧命名空间
    kubectl delete namespace $TEST_NS 2>/dev/null || true
    sleep 2
    kubectl create namespace $TEST_NS

    echo -e "${BLUE}== 安全规则 (20 条) ==${NC}"
    test_rule "runAsPrivileged" "security/run-as-privileged-fail.yaml" "deny"
    test_rule "privilegeEscalationAllowed" "security/privilege-escalation-fail.yaml" "deny"
    test_rule "runAsRootAllowed" "security/run-as-root-fail.yaml" "allow"
    test_rule "notReadOnlyRootFilesystem" "security/not-readonly-filesystem-fail.yaml" "allow"
    test_rule "linuxHardening" "security/linux-hardening-fail.yaml" "deny"
    test_rule "insecureCapabilities" "security/insecure-capabilities-fail.yaml" "allow"
    test_rule "dangerousCapabilities" "security/dangerous-capabilities-fail.yaml" "deny"
    test_rule "hostPIDSet" "security/host-pid-fail.yaml" "deny"
    test_rule "hostIPCSet" "security/host-ipc-fail.yaml" "deny"
    test_rule "hostNetworkSet" "security/host-network-fail.yaml" "allow"
    test_rule "hostPortSet" "security/host-port-fail.yaml" "allow"
    test_rule "sensitiveContainerEnvVar" "security/sensitive-env-var-fail.yaml" "deny"
    test_rule "automountServiceAccountToken" "security/automount-token-fail.yaml" "allow"
    test_rule "sensitiveConfigmapContent" "security/sensitive-configmap-fail.yaml" "deny"
    test_rule "tlsSettingsMissing" "security/tls-settings-missing-fail.yaml" "allow"
    test_rule "clusterrolePodExecAttach" "security/clusterrole-exec-fail.yaml" "deny"
    test_rule "rolePodExecAttach" "security/role-exec-fail.yaml" "deny"
    test_rule "clusterrolebindingPodExecAttach" "security/clusterrolebinding-exec-fail.yaml" "deny"
    test_rule "rolebindingRolePodExecAttach" "security/rolebinding-role-exec-fail.yaml" "deny"
    test_rule "rolebindingClusterRolePodExecAttach" "security/rolebinding-clusterrole-exec-fail.yaml" "deny"
    test_rule "clusterrolebindingClusterAdmin" "security/clusterrolebinding-admin-fail.yaml" "deny"
    test_rule "rolebindingClusterAdminClusterRole" "security/rolebinding-clusteradmin-clusterrole-fail.yaml" "deny"
    test_rule "rolebindingClusterAdminRole" "security/rolebinding-clusteradmin-role-fail.yaml" "deny"

    echo ""
    echo -e "${BLUE}== 效率规则 (4 条) ==${NC}"
    test_rule "cpuRequestsMissing" "efficiency/cpu-requests-missing-fail.yaml" "allow"
    test_rule "memoryRequestsMissing" "efficiency/memory-requests-missing-fail.yaml" "allow"
    test_rule "cpuLimitsMissing" "efficiency/cpu-limits-missing-fail.yaml" "allow"
    test_rule "memoryLimitsMissing" "efficiency/memory-limits-missing-fail.yaml" "allow"

    echo ""
    echo -e "${BLUE}== 可靠性规则 (13 条) ==${NC}"
    test_rule "readinessProbeMissing" "reliability/readiness-probe-missing-fail.yaml" "allow"
    test_rule "livenessProbeMissing" "reliability/liveness-probe-missing-fail.yaml" "allow"
    test_rule "tagNotSpecified-latest" "reliability/tag-not-specified-fail.yaml" "deny"
    test_rule "tagNotSpecified-notag" "reliability/tag-not-specified-fail-2.yaml" "deny"
    test_rule "pullPolicyNotAlways" "reliability/pull-policy-not-always-fail.yaml" "allow"
    test_rule "priorityClassNotSet" "reliability/priority-class-not-set-fail.yaml" "allow"
    test_rule "deploymentMissingReplicas" "reliability/deployment-single-replica-fail.yaml" "allow"
    test_rule "metadataAndInstanceMismatched" "reliability/metadata-instance-mismatch-fail.yaml" "allow"
    test_rule "topologySpreadConstraint" "reliability/topology-spread-missing-fail.yaml" "allow"
    test_rule "hpaMaxAvailability" "reliability/hpa-max-availability-fail.yaml" "allow"
    test_rule "hpaMinAvailability" "reliability/hpa-min-availability-fail.yaml" "allow"

    # 显示结果
    echo ""
    echo "=========================================="
    echo "测试结果汇总"
    echo "=========================================="
    echo ""
    echo -e "总计: ${BLUE}$total${NC} 条规则"
    echo -e "通过: ${GREEN}$passed${NC} 条"
    echo -e "失败: ${RED}$failed${NC} 条"
    echo -e "成功率: ${BLUE}$((passed * 100 / total))%${NC}"
    echo ""

    # 详细结果
    echo -e "${BLUE}=== 详细结果 ===${NC}"
    for result in "${results[@]}"; do
        IFS='|' read -r status rule msg <<< "$result"
        case $status in
            PASS) echo -e "${GREEN}✓${NC} $rule - $msg" ;;
            FAIL) echo -e "${RED}✗${NC} $rule - $msg" ;;
            SKIP) echo -e "${YELLOW}⊘${NC} $rule - $msg" ;;
        esac
    done

    # 显示 webhook 日志
    show_webhook_logs

    # 清理
    echo ""
    log_info "清理测试资源..."
    kubectl delete namespace $TEST_NS 2>/dev/null || true

    # 返回状态
    if [ $failed -eq 0 ]; then
        log_info "所有规则测试完成！"
        exit 0
    else
        log_fail "$failed 条规则测试失败"
        exit 1
    fi
}

main "$@"
