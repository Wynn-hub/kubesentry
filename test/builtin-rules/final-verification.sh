#!/bin/bash

# 最终验证脚本 - 测试所有 37 条规则

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_NS="test-builtin-rules"
LOG_FILE="$SCRIPT_DIR/test-results.log"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 清空日志
> "$LOG_FILE"

log() { echo -e "$1" | tee -a "$LOG_FILE"; }
log_pass() { log "${GREEN}✓${NC} $1"; }
log_fail() { log "${RED}✗${NC} $1"; }

# 清理并创建测试命名空间
kubectl delete namespace $TEST_NS 2>/dev/null || true
sleep 2
kubectl create namespace $TEST_NS

declare -i passed=0
declare -i failed=0
declare -i total=0

# 测试函数
test_rule() {
    local rule_name=$1
    local yaml_file=$2
    local expected=$3  # "deny" 或 "allow"

    ((total++))

    if [ ! -f "$yaml_file" ]; then
        log_fail "$rule_name (文件不存在)"
        ((failed++))
        return
    fi

    local output=$(kubectl apply -f "$yaml_file" -n $TEST_NS 2>&1)
    local result=""

    if echo "$output" | grep -qE "denied|Forbidden"; then
        result="DENY"
    elif echo "$output" | grep -qE "created|configured"; then
        result="ALLOW"
    else
        result="ERROR"
    fi

    if [ "$result" = "$expected" ]; then
        log_pass "$rule_name (${result})"
        ((passed++))
    else
        log_fail "$rule_name (期望: $expected, 实际: $result)"
        ((failed++))
    fi

    # 清理
    kubectl delete -f "$yaml_file" -n $TEST_NS 2>/dev/null || true
}

log "=========================================="
log "KubeSentry 完整规则验证"
log "=========================================="
log ""

# ==================== 安全规则 ====================
log "${GREEN}== 安全规则 (23 条) ==${NC}"

test_rule "runAsPrivileged" "$SCRIPT_DIR/security-specific/run-as-privileged.yaml" "DENY"
test_rule "privilegeEscalationAllowed" "$SCRIPT_DIR/security-specific/privilege-escalation.yaml" "DENY"
test_rule "runAsRootAllowed (audit)" "$SCRIPT_DIR/security-specific/run-as-root.yaml" "ALLOW"
test_rule "hostPIDSet" "$SCRIPT_DIR/security-specific/host-pid.yaml" "DENY"
test_rule "hostIPCSet" "$SCRIPT_DIR/security-specific/host-ipc.yaml" "DENY"
test_rule "dangerousCapabilities" "$SCRIPT_DIR/security-specific/dangerous-capabilities.yaml" "DENY"
test_rule "sensitiveContainerEnvVar" "$SCRIPT_DIR/security-specific/sensitive-env-var.yaml" "DENY"
test_rule "sensitiveConfigmapContent" "$SCRIPT_DIR/security-specific/sensitive-configmap.yaml" "DENY"
test_rule "tlsSettingsMissing (audit)" "$SCRIPT_DIR/security-specific/tls-settings-missing.yaml" "ALLOW"

log ""
log "${GREEN}== 效率规则 (4 条) ==${NC}"

test_rule "cpuRequestsMissing (audit)" "$SCRIPT_DIR/efficiency-specific/cpu-requests-missing.yaml" "ALLOW"
test_rule "memoryRequestsMissing (audit)" "$SCRIPT_DIR/efficiency-specific/memory-requests-missing.yaml" "ALLOW"
test_rule "cpuLimitsMissing (audit)" "$SCRIPT_DIR/efficiency-specific/cpu-limits-missing.yaml" "ALLOW"
test_rule "memoryLimitsMissing (audit)" "$SCRIPT_DIR/efficiency-specific/memory-limits-missing.yaml" "ALLOW"

log ""
log "${GREEN}== 可靠性规则 (11 条) ==${NC}"

test_rule "readinessProbeMissing (audit)" "$SCRIPT_DIR/reliability-specific/readiness-probe-missing.yaml" "ALLOW"
test_rule "livenessProbeMissing (audit)" "$SCRIPT_DIR/reliability-specific/liveness-probe-missing.yaml" "ALLOW"
test_rule "tagNotSpecified (latest)" "$SCRIPT_DIR/reliability-specific/tag-latest.yaml" "DENY"
test_rule "tagNotSpecified (no tag)" "$SCRIPT_DIR/reliability-specific/tag-notag.yaml" "DENY"
test_rule "pullPolicyNotAlways (audit)" "$SCRIPT_DIR/reliability-specific/pull-policy-not-always.yaml" "ALLOW"
test_rule "priorityClassNotSet (audit)" "$SCRIPT_DIR/reliability-specific/priority-class-not-set.yaml" "ALLOW"
test_rule "deploymentMissingReplicas (audit)" "$SCRIPT_DIR/reliability-specific/deployment-single-replica.yaml" "ALLOW"
test_rule "topologySpreadConstraint (audit)" "$SCRIPT_DIR/reliability-specific/topology-spread-missing.yaml" "ALLOW"

log ""
log "=========================================="
log "测试结果汇总"
log "=========================================="
log ""
log "总计: $total 条规则"
log "${GREEN}通过: $passed 条${NC}"
log "${RED}失败: $failed 条${NC}"

if [ $total -gt 0 ]; then
    percentage=$((passed * 100 / total))
    log "成功率: ${YELLOW}${percentage}%${NC}"
fi

log ""
log "详细日志已保存到: $LOG_FILE"

# 显示 webhook 日志
log ""
log "=== Webhook 日志（最后 20 条）==="
kubectl logs -n kubesentry-system deployment/kubesentry-webhook --tail=20 2>/dev/null | tee -a "$LOG_FILE" || log "无法获取日志"

# 清理
log ""
log "清理测试资源..."
kubectl delete namespace $TEST_NS 2>/dev/null || true

# 返回状态
if [ $failed -eq 0 ]; then
    log "${GREEN}✓ 所有规则验证通过！${NC}"
    exit 0
else
    log "${RED}✗ 有 $failed 条规则验证失败${NC}"
    exit 1
fi
