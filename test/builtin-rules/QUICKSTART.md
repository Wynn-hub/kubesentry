# 快速开始 - KubeSentry 内置规则测试

## 5分钟快速测试

### 前置条件

- ✓ Kubernetes 集群已运行
- ✓ KubeSentry 已部署在 `kubesentry-system` 命名空间

验证 kubesentry 部署：
```bash
kubectl get deployment -n kubesentry-system
# 应该看到 webhook 和 operator
```

### 第 1 步：运行测试

```bash
cd test/builtin-rules
bash test-builtin-rules.sh
```

脚本会自动：
1. 验证 kubesentry 已部署
2. 创建测试命名空间
3. 启用所有 37 条内置规则
4. 运行所有规则测试
5. 清理测试资源

### 预期输出

```
======================================
KubeSentry 内置规则测试
======================================

[INFO] 检查 kubesentry 是否已部署...
[INFO] ✓ kubesentry 已正确部署
[INFO] 设置测试命名空间...
[INFO] ✓ 命名空间 test-builtin-rules 已创建
[INFO] 启用内置规则...
[INFO] ✓ 已创建 37 条 Policy

[INFO] === 测试安全规则 ===
  测试 runAsPrivileged (run-as-privileged-fail)... ✓ 正确被拒绝
  测试 privilegeEscalationAllowed (privilege-escalation-fail)... ✓ 正确被拒绝
  ...
安全规则测试: 20 passed, 0 failed

[INFO] === 测试效率规则 ===
  ...
效率规则测试: 4 passed, 0 failed

[INFO] === 测试可靠性规则 ===
  ...
可靠性规则测试: 13 passed, 0 failed

======================================
[INFO] ✓ 所有测试通过！
```

## 单个规则测试

### 测试特定规则

```bash
# 查看某个规则的测试YAML
cat security/run-as-privileged-fail.yaml

# 应用测试资源
kubectl apply -f security/run-as-privileged-fail.yaml

# 预期：admission webhook 拒绝
# Error from server (Forbidden): ...
# admission webhook "validate.kubesentry.io" denied the request
```

### 查看 webhook 日志

```bash
kubectl logs -n kubesentry-system deployment/webhook -f
```

## 测试文件目录

```
test/builtin-rules/
├── security/            # 20 条安全规则
├── efficiency/          # 4 条效率规则  
└── reliability/         # 13 条可靠性规则
```

每个 `*-fail.yaml` 文件包含会触发对应规则的资源。

## 规则覆盖

| 分类 | 数量 | 示例 |
|------|------|------|
| 安全 | 20 | runAsPrivileged, hostPID, dangerousCapabilities |
| 效率 | 4 | cpuRequests, memoryLimits |
| 可靠性 | 13 | readinessProbe, tagNotSpecified, replicas |
| **总计** | **37** | |

## 故障排除

### 测试失败怎么办？

1. 检查 kubesentry 日志
   ```bash
   kubectl logs -n kubesentry-system deployment/webhook -f
   kubectl logs -n kubesentry-system deployment/operator -f
   ```

2. 验证 Policy 状态
   ```bash
   kubectl get policy -n kubesentry-system | head -10
   kubectl describe policy runAsPrivileged -n kubesentry-system
   ```

3. 检查 ValidatingWebhookConfiguration
   ```bash
   kubectl get validatingwebhookconfigurations kubesentry -o yaml
   ```

### 特定规则不工作？

查看完整 README：
```bash
cat README.md
```

## 下一步

- 📖 完整文档：[README.md](README.md)
- 🛠️ 开发指南：[../../DEVELOPMENT.md](../../DEVELOPMENT.md)
- 📝 规则编写：[../../examples/README.md](../../examples/README.md)

---

**总结**：39 个测试 YAML 文件 + 1 个测试脚本 = 完整的规则验证框架 ✓
