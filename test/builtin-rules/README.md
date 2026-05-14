# KubeSentry 内置规则测试框架

本目录包含了对 KubeSentry 的 37 条内置策略的完整测试框架。

## 目录结构

```
test/builtin-rules/
├── README.md                          # 本文件
├── generate-test-yamls.sh             # 生成所有测试YAML的脚本
├── test-builtin-rules.sh              # 运行测试的脚本
├── enable-builtin-policies.yaml       # 启用所有内置规则的PolicyGroup
└── {category}/                        # 按分类组织的测试YAML
    ├── security/                      # 安全相关规则 (20条)
    ├── efficiency/                    # 效率相关规则 (4条)
    └── reliability/                   # 可靠性相关规则 (13条)
```

## 37 条内置规则

### 安全规则 (20条)

1. **runAsPrivileged** — Pod运行特权模式 (enforce)
2. **privilegeEscalationAllowed** — 允许特权提升 (enforce)
3. **runAsRootAllowed** — 以root运行 (audit)
4. **notReadOnlyRootFilesystem** — 根文件系统可写 (audit)
5. **linuxHardening** — 缺少Linux安全加固 (enforce)
6. **insecureCapabilities** — 不安全的capability (audit)
7. **dangerousCapabilities** — 危险的capability (enforce)
8. **hostPIDSet** — 使用hostPID (enforce)
9. **hostIPCSet** — 使用hostIPC (enforce)
10. **hostNetworkSet** — 使用hostNetwork (audit)
11. **hostPortSet** — 使用hostPort (audit)
12. **sensitiveContainerEnvVar** — 敏感的环境变量 (enforce)
13. **automountServiceAccountToken** — 自动挂载SA令牌 (audit)
14. **sensitiveConfigmapContent** — ConfigMap包含敏感内容 (enforce)
15. **tlsSettingsMissing** — Ingress缺少TLS (audit)
16. **clusterrolePodExecAttach** — ClusterRole允许pod/exec (enforce)
17. **rolePodExecAttach** — Role允许pod/attach (enforce)
18. **clusterrolebindingPodExecAttach** — ClusterRoleBinding指向危险ClusterRole (enforce)
19. **rolebindingRolePodExecAttach** — RoleBinding指向危险Role (enforce)
20. **rolebindingClusterRolePodExecAttach** — RoleBinding指向危险ClusterRole (enforce)
21. **clusterrolebindingClusterAdmin** — ClusterRoleBinding到cluster-admin (enforce)
22. **rolebindingClusterAdminClusterRole** — RoleBinding到cluster-admin ClusterRole (enforce)
23. **rolebindingClusterAdminRole** — RoleBinding到通配符Role (enforce)

### 效率规则 (4条)

1. **cpuRequestsMissing** — 缺少CPU请求 (audit)
2. **memoryRequestsMissing** — 缺少内存请求 (audit)
3. **cpuLimitsMissing** — 缺少CPU限制 (audit)
4. **memoryLimitsMissing** — 缺少内存限制 (audit)

### 可靠性规则 (13条)

1. **readinessProbeMissing** — 缺少就绪探针 (audit)
2. **livenessProbeMissing** — 缺少存活性探针 (audit)
3. **tagNotSpecified** — 镜像标签为latest或未指定 (enforce)
4. **pullPolicyNotAlways** — imagePullPolicy不是Always (audit)
5. **priorityClassNotSet** — 未设置priorityClassName (audit)
6. **deploymentMissingReplicas** — Deployment仅有1个副本 (audit)
7. **metadataAndInstanceMismatched** — 标签和元数据不匹配 (audit)
8. **topologySpreadConstraint** — 缺少拓扑约束 (audit)
9. **hpaMaxAvailability** — HPA maxReplicas <= minReplicas (audit)
10. **hpaMinAvailability** — HPA minReplicas <= 1 (audit)

## 使用指南

### 先决条件

- Kubernetes集群 (1.19+)
- kubectl 配置好
- KubeSentry 已在集群中部署
  ```bash
  # 参考 /docs/DEVELOPMENT.md 或 README.md 了解部署步骤
  kubectl get deployment -n kubesentry-system
  ```

### 快速开始

#### 1. 生成测试YAML文件

```bash
cd test/builtin-rules
bash generate-test-yamls.sh
```

这会在 `security/`, `efficiency/`, `reliability/` 目录中生成所有测试YAML。

#### 2. 运行完整测试套件

```bash
bash test-builtin-rules.sh
```

脚本会：
- ✓ 检查 kubesentry 是否已部署
- ✓ 创建测试命名空间 `test-builtin-rules`
- ✓ 应用 PolicyGroup 启用所有规则
- ✓ 运行所有测试
- ✓ 验证规则是否正确触发或允许
- ✓ 清理测试资源

#### 3. 单独测试特定规则

```bash
# 测试 runAsPrivileged 规则
kubectl apply -f security/run-as-privileged-fail.yaml

# 应该看到 admission webhook denied 错误
# Error from server (Forbidden): error when creating "security/run-as-privileged-fail.yaml":
# admission webhook "validate.kubesentry.io" denied the request:
# runAsPrivileged: container "privileged-container" must not run as privileged
```

### 测试YAML文件说明

每个测试YAML文件都设计用来触发或不触发特定规则：

#### "fail" YAML（触发规则）
- 文件名: `{rule-name}-fail.yaml`
- 用途: 包含会违反规则的配置
- 期望结果: 
  - `enforce` 模式: **被 admission webhook 拒绝**
  - `audit` 模式: **被接受，但返回 warning**

示例：
```yaml
# security/run-as-privileged-fail.yaml
apiVersion: v1
kind: Pod
metadata:
  name: privileged-pod
spec:
  containers:
  - name: app
    image: nginx:1.19
    securityContext:
      privileged: true  # ← 触发 runAsPrivileged 规则
```

#### "pass" YAML（不触发规则，稍后补充）
- 文件名: `{rule-name}-pass.yaml`
- 用途: 包含符合规则的配置
- 期望结果: **被接受**

## PolicyGroup 配置

`enable-builtin-policies.yaml` 文件定义了一个 PolicyGroup，启用所有 37 条内置规则：

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: PolicyGroup
metadata:
  name: builtin-rules-test
  namespace: kubesentry-system
spec:
  policies:
  - key: runAsPrivileged
    enforceMode: enforce
  - key: cpuRequestsMissing
    enforceMode: audit
  # ... 其他 34 条规则
```

### 自定义规则模式

编辑 `enable-builtin-policies.yaml` 中的 `enforceMode`:
- `enforce`: 违反规则时拒绝请求（默认）
- `audit`: 违反规则时允许但记录 warning

示例：让某个规则以审计模式运行
```yaml
- key: runAsPrivileged
  enforceMode: audit  # 改为 audit 模式
```

然后重新应用：
```bash
kubectl apply -f enable-builtin-policies.yaml
```

## 测试结果解读

### 成功的测试输出

```
====================================
KubeSentry 内置规则测试
====================================

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

### 失败的测试输出

```
[ERROR] 测试 runAsPrivileged (run-as-privileged-fail)... ✗ 不应该被接受

安全规则测试: 19 passed, 1 failed
```

这表示规则没有正确触发。检查：
1. Policy 是否正确部署：`kubectl get policy -n kubesentry-system`
2. Policy 是否处于 `Ready` 状态
3. webhook 日志：`kubectl logs -n kubesentry-system deployment/webhook`

## 调试指南

### 查看 webhook 日志

```bash
kubectl logs -n kubesentry-system deployment/webhook -f
```

### 查看已部署的 Policy

```bash
# 列出所有 Policy
kubectl get policy -n kubesentry-system

# 查看特定 Policy 的详情
kubectl describe policy runAsPrivileged -n kubesentry-system

# 查看 Policy 的 Rego 内容
kubectl get policy runAsPrivileged -n kubesentry-system -o jsonpath='{.spec.rego}'
```

### 查看 ValidatingWebhookConfiguration

```bash
# 查看 webhook 配置
kubectl get validatingwebhookconfigurations kubesentry -o yaml

# 验证规则是否已被自动填充
kubectl get validatingwebhookconfigurations kubesentry \
  -o jsonpath='{.webhooks[0].rules}' | jq
```

### 手动测试单个规则

```bash
# 创建会触发规则的资源
kubectl apply -f security/run-as-privileged-fail.yaml -n test-builtin-rules

# 查看 admission review 日志（在webhook容器中）
kubectl logs -n kubesentry-system deployment/webhook --tail=20
```

### 规则未被触发的常见原因

1. **Policy 未处于 Ready 状态**
   ```bash
   kubectl get policy -n kubesentry-system -o wide
   # 检查 STATUS 列，应该是 "Ready"
   ```

2. **ValidatingWebhookConfiguration 的规则未更新**
   ```bash
   kubectl get validatingwebhookconfigurations kubesentry -o yaml | grep -A5 rules:
   # 应该包含所有资源类型（pods, deployments, services, 等）
   ```

3. **Rego 编译错误**
   ```bash
   kubectl describe policy runAsPrivileged -n kubesentry-system
   # 查看 Status.Message 字段中是否有错误信息
   ```

4. **命名空间标签**
   - 确保 `test-builtin-rules` 命名空间存在
   - 某些规则可能有特定的命名空间要求

## 添加新规则的测试

如果添加了新的内置规则：

1. 在相应分类目录中创建 `{rule-name}-fail.yaml`
2. 更新 `enable-builtin-policies.yaml` 中的规则列表
3. 在 `test-builtin-rules.sh` 中添加相应的测试函数
4. 运行脚本验证

示例：
```bash
# 在 security/ 目录中创建新规则的测试
cat > security/my-new-rule-fail.yaml << 'EOF'
apiVersion: v1
kind: Pod
metadata:
  name: my-test-pod
  namespace: test-builtin-rules
spec:
  containers:
  - name: app
    image: nginx:1.19
    # 会触发 myNewRule 的配置
EOF
```

## 性能考虑

- 首次运行需要 5-10 秒来等待 Policy 创建和 webhook 初始化
- 每个测试资源创建/删除约需 1-2 秒
- 完整测试套件通常耗时 2-5 分钟

## 故障排除

### webhook 响应缓慢

webhook 可能在处理大量 Policy 时响应变慢。增加等待时间：
```bash
# 修改 test-builtin-rules.sh 中的 sleep 值
sleep 10  # 从 5 改为 10
```

### Policy 创建失败

检查 operator 日志：
```bash
kubectl logs -n kubesentry-system deployment/operator -f
```

常见错误：
- Rego 编译错误：检查 Policy 的 Rego 语法
- 资源不足：增加 operator Pod 的资源限制
- API 速率限制：减少并发 Policy 创建

### webhook 无法验证资源

确保 ValidatingWebhookConfiguration 已正确配置：
```bash
kubectl get validatingwebhookconfigurations kubesentry -o yaml
```

检查：
- `rules` 字段不为空
- `clientConfig.caBundle` 已设置
- `clientConfig.service` 指向正确的 webhook Service

## 参考资源

- [KubeSentry README](../../README.md) — 项目概述
- [DEVELOPMENT.md](../../DEVELOPMENT.md) — 开发指南
- [CONTRIBUTING.md](../../CONTRIBUTING.md) — 贡献指南
- [OPA Rego 文档](https://www.openpolicyagent.org/docs/latest/policy-language/)
- [Kubernetes Admission Webhook 文档](https://kubernetes.io/docs/reference/access-authn-authz/webhook/)

## 许可证

Apache License 2.0 — 详见项目根目录的 LICENSE 文件
