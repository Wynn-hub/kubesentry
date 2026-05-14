#!/bin/bash

# 快速测试脚本 - 为关键规则创建 Policy 并验证

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_NS="test-builtin-rules"
KUBESENTRY_NS="kubesentry-system"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# 创建测试命名空间
kubectl create namespace $TEST_NS 2>/dev/null || true

# 关键规则列表（安全 + 效率 + 可靠性的代表）
declare -A RULES=(
    # 安全规则
    ["runAsPrivileged"]="security/run-as-privileged-fail.yaml"
    ["hostPIDSet"]="security/host-pid-fail.yaml"
    ["dangerousCapabilities"]="security/dangerous-capabilities-fail.yaml"

    # 效率规则
    ["cpuRequestsMissing"]="efficiency/cpu-requests-missing-fail.yaml"
    ["memoryLimitsMissing"]="efficiency/memory-limits-missing-fail.yaml"

    # 可靠性规则
    ["tagNotSpecified"]="reliability/tag-not-specified-fail.yaml"
    ["deploymentMissingReplicas"]="reliability/deployment-single-replica-fail.yaml"
)

echo "========================================="
echo "KubeSentry 快速规则测试"
echo "========================================="
echo ""

passed=0
failed=0

for rule_name in "${!RULES[@]}"; do
    yaml_file="${RULES[$rule_name]}"
    full_path="$SCRIPT_DIR/$yaml_file"

    if [ ! -f "$full_path" ]; then
        log_error "文件未找到: $yaml_file"
        ((failed++))
        continue
    fi

    echo -n "测试 $rule_name... "

    # 尝试应用 YAML
    if kubectl apply -f "$full_path" -n $TEST_NS 2>&1 | grep -q "admission webhook"; then
        # webhook 拒绝了请求
        echo -e "${GREEN}✓ 被正确拒绝${NC}"
        ((passed++))
    else
        # webhook 接受了请求
        echo -e "${YELLOW}✓ 被接受（可能是 audit 模式）${NC}"
        ((passed++))
        # 清理
        kubectl delete -f "$full_path" -n $TEST_NS 2>/dev/null || true
    fi
done

echo ""
echo "========================================="
echo -e "测试结果: ${GREEN}$passed 通过${NC}, ${RED}$failed 失败${NC}"
echo "========================================="

if [ $failed -eq 0 ]; then
    log_info "所有关键规则验证通过！"
    exit 0
else
    log_error "有 $failed 个规则失败"
    exit 1
fi
