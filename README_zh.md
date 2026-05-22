# KubeSentry

[English](README.md) | 中文

[![Go Version](https://img.shields.io/badge/go-1.26+-blue)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-green)](LICENSE)
[![Docker Hub](https://img.shields.io/docker/pulls/wynnhub/kubesentry-webhook?label=webhook%20pulls)](https://hub.docker.com/r/wynnhub/kubesentry-webhook)
[![Docker Hub](https://img.shields.io/docker/pulls/wynnhub/kubesentry-operator?label=operator%20pulls)](https://hub.docker.com/r/wynnhub/kubesentry-operator)

基于 OPA/Rego 策略引擎的 Kubernetes Validating Admission Webhook，通过 CRD 管理策略生命周期，支持内置策略组、版本控制与回滚。

## 快速开始

```bash
helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --namespace kubesentry-system \
  --create-namespace
```

无需登录——镜像和 Helm Chart 均在 Docker Hub 公开发布。Helm pre-install Job 会自动生成 TLS 证书并注入 `ValidatingWebhookConfiguration`。

## 架构

两个独立的 Go 二进制程序共享相同的 CRD API 类型：

```
┌────────────────────────────────────────────────────────────────┐
│                         Kubernetes 集群                         │
│                                                                 │
│  ┌──────────────────┐      ┌────────────────────────────────┐  │
│  │  kubesentry-     │      │     kubesentry-operator        │  │
│  │  webhook         │      │                                │  │
│  │                  │      │  PolicyGroupReconciler         │  │
│  │  - OPA 评估器    │      │  - 解析 byName + bySelector    │  │
│  │  - 策略缓存      │      │  - 计算每条成员的生效模式      │  │
│  │  - /validate     │      │  - 写入 status.resolvedPolicies│  │
│  │  - /healthz      │      │                                │  │
│  │  - /readyz       │      │  PolicyReconciler              │  │
│  └────────┬─────────┘      │  - 校验 Rego                   │  │
│           │                │  - 创建 PolicyVersion          │  │
│           │ 监听           │  - 处理回滚                    │  │
│           ▼                │                                │  │
│  ┌──────────────────┐      │  WebhookConfigReconciler       │  │
│  │   Policy CRD     │      │  - 聚合规则                    │  │
│  │   （集群级别）    │      │  - 更新 VWC                    │  │
│  └──────────────────┘      └────────────────────────────────┘  │
│                                                                 │
│  ┌──────────────────┐      ┌────────────────────────────────┐  │
│  │  PolicyGroup CRD │      │  PolicyVersion CRD             │  │
│  │  （集群级别）     │      │  （不可变快照）                 │  │
│  └──────────────────┘      └────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
```

## 功能特性

- **嵌入式 OPA 引擎** — 直接内嵌 OPA，无需 sidecar
- **CRD 策略管理** — 以 `Policy` Kubernetes 资源定义策略
- **内置策略组** — 37 条精选规则，覆盖 Security、Efficiency、Reliability 三个分组，安装时作为独立 `Policy` CR 部署
- **策略组管理** — 支持分组级别和单条策略的独立开关
- **自定义策略组** — 创建自己的 `PolicyGroup` CRD 引用内置或自定义 `Policy`；`PolicyGroup` 是纯引用对象，不拥有也不创建 `Policy` CR
- **结构化违规消息** — 拒绝响应包含 `[组/key] 消息` 和描述字段；audit 模式违规以 `AdmissionResponse.Warnings` 返回
- **双执行模式** — `enforce`（拦截）或 `audit`（仅记录/告警）
- **版本控制** — 每次策略变更自动创建不可变的 `PolicyVersion` 快照
- **一键回滚** — 设置 `spec.rollbackTo.version` 即可还原任意历史版本
- **动态规则同步** — Operator 自动根据 Ready 状态的策略更新 `ValidatingWebhookConfiguration`
- **自签 TLS** — Helm 安装时通过 pre-install Job 自动生成证书并注入 `caBundle`
- **并发评估** — 多条策略并发执行，超时时间 5 秒
- **Leader 选举** — Operator 支持高可用 Leader 选举
- **多平台镜像** — 同时支持 `linux/amd64` 和 `linux/arm64`

## 内置策略组

KubeSentry 内置 37 条策略，分为三个组，默认全部启用：

### Security（23 条）

| Key | 默认模式 | 说明 |
|---|---|---|
| `runAsPrivileged` | enforce | 禁止容器以特权模式运行 |
| `privilegeEscalationAllowed` | enforce | 禁止 `allowPrivilegeEscalation: true` |
| `runAsRootAllowed` | audit | 未设置 `runAsNonRoot` 时告警 |
| `notReadOnlyRootFilesystem` | audit | 未设置 `readOnlyRootFilesystem` 时告警 |
| `linuxHardening` | enforce | 要求至少配置 seccompProfile、seLinuxOptions 或 capabilities.drop 之一 |
| `insecureCapabilities` | audit | 添加不安全 capability 时告警 |
| `dangerousCapabilities` | enforce | 禁止危险 capability（SYS_ADMIN、NET_ADMIN 等） |
| `hostPIDSet` | enforce | 禁止 `hostPID: true` |
| `hostIPCSet` | enforce | 禁止 `hostIPC: true` |
| `hostNetworkSet` | audit | 使用 `hostNetwork: true` 时告警 |
| `hostPortSet` | audit | 设置 `hostPort` 时告警 |
| `sensitiveContainerEnvVar` | enforce | 禁止环境变量名包含敏感关键词 |
| `automountServiceAccountToken` | audit | 自动挂载服务账号令牌时告警 |
| `sensitiveConfigmapContent` | enforce | 禁止 ConfigMap 包含疑似敏感内容的键 |
| `tlsSettingsMissing` | audit | Ingress 未配置 TLS 时告警 |
| `clusterrolePodExecAttach` | enforce | 禁止 ClusterRole 授予 pods/exec 或 pods/attach |
| `rolePodExecAttach` | enforce | 禁止 Role 授予 pods/exec 或 pods/attach |
| `clusterrolebindingPodExecAttach` | enforce | 禁止 ClusterRoleBinding 引用名称包含 exec/attach 的角色 |
| `rolebindingRolePodExecAttach` | enforce | 禁止 RoleBinding 引用名称包含 exec/attach 的 Role |
| `rolebindingClusterRolePodExecAttach` | enforce | 禁止 RoleBinding 引用名称包含 exec/attach 的 ClusterRole |
| `clusterrolebindingClusterAdmin` | enforce | 禁止 ClusterRoleBinding 绑定 cluster-admin |
| `rolebindingClusterAdminClusterRole` | enforce | 禁止 RoleBinding 绑定 cluster-admin ClusterRole |
| `rolebindingClusterAdminRole` | enforce | 禁止 RoleBinding 绑定名为 cluster-admin 的 Role |

### Efficiency（4 条）

| Key | 默认模式 | 说明 |
|---|---|---|
| `cpuRequestsMissing` | audit | 未设置 CPU requests 时告警 |
| `memoryRequestsMissing` | audit | 未设置内存 requests 时告警 |
| `cpuLimitsMissing` | audit | 未设置 CPU limits 时告警 |
| `memoryLimitsMissing` | audit | 未设置内存 limits 时告警 |

### Reliability（10 条）

| Key | 默认模式 | 说明 |
|---|---|---|
| `readinessProbeMissing` | audit | 未配置 readiness probe 时告警 |
| `livenessProbeMissing` | audit | 未配置 liveness probe 时告警 |
| `tagNotSpecified` | enforce | 禁止镜像不带 tag 或使用 `:latest` |
| `pullPolicyNotAlways` | audit | imagePullPolicy 非 Always 时告警 |
| `priorityClassNotSet` | audit | 未设置 priorityClassName 时告警 |
| `deploymentMissingReplicas` | audit | Deployment 副本数不足 2 时告警 |
| `metadataAndInstanceMismatched` | audit | `metadata.name` 与 `app.kubernetes.io/instance` 不一致时告警 |
| `topologySpreadConstraint` | audit | 未配置拓扑分布约束时告警 |
| `hpaMaxAvailability` | audit | HPA maxReplicas ≤ minReplicas 时告警 |
| `hpaMinAvailability` | audit | HPA minReplicas ≤ 1 时告警 |

### 自定义内置策略

通过 Helm values 关闭整组、单条策略，或覆盖执行模式：

```yaml
# 禁用整个内置策略组
builtinGroups:
  efficiency:
    enabled: false

# 启用/禁用或覆盖单条内置策略（key 使用 kebab-case 的 Policy 名）
builtinPolicies:
  host-network-set:
    enabled: false         # 完全移除该策略
  run-as-root-allowed:
    mode: enforce          # 由 audit 升级为 enforce

# 覆盖所有内置组共用的命名空间排除列表
builtinNamespaceSelector:
  matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: NotIn
      values:
        - kube-system
        - kube-public
        - my-exempt-namespace
```

> ⚠️ **bySelector 动态再捕获。** 内置组通过两种方式绑定成员：
> `byName`（显式列表）**和** `bySelector`（`kubesentry.io/category=<group>`）。
> 设置 `builtinPolicies.<name>.enabled=false` 只阻止 Helm 渲染对应的
> Policy CR — 如果后续有人 apply 一个同名且带有该 category 标签的 CR，
> 下一次 PolicyGroup reconcile 时 `bySelector` 会再次把它纳入组。
> 若要永久把某条策略排除出组，需同时去掉未来副本上的
> `kubesentry.io/category` 标签，或干脆禁用整个组。

## PolicyGroup CRD

`PolicyGroup` 是**纯引用对象** — 它**不拥有也不创建** `Policy` CR。你可以建立自己的组，引用内置或自定义的 Policy：

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: PolicyGroup
metadata:
  name: my-policies
spec:
  enabled: true
  displayName: "我的自定义策略"
  namespaceSelector:
    matchExpressions:
      - key: kubernetes.io/metadata.name
        operator: NotIn
        values: [kube-system, kube-public]
  policies:
    byName:
      # 按名称引用内置或自定义 Policy，可选地覆盖执行模式
      - name: run-as-privileged
        enforcementMode: enforce
      - name: no-debug-containers   # 你自己的 Policy CR
    bySelector:
      # 动态纳入所有带 kubesentry.io/category=custom 标签的 Policy
      matchLabels:
        kubesentry.io/category: custom
  selectorEnforcementMode: audit   # 应用到所有 bySelector 命中的成员
```

**逐请求取最严格模式。** 当同一次准入请求被多个启用的 PolicyGroup 命中（其 `namespaceSelector` 都匹配该命名空间），且这些组对同一条 Policy 给出了不同的生效模式时，Webhook 会取最严格的（enforce > audit）。

### 自定义 Policy CR

定义一个独立的 `Policy` CR 并通过组引用：

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: no-debug-containers
  labels:
    kubesentry.io/category: custom   # 会被 bySelector 类型的组捕获
spec:
  enforcementMode: enforce
  match:
    operations: [CREATE, UPDATE]
    resources:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
  rego: |
    package kubesentry
    deny[msg] {
      c := input.request.object.spec.containers[_]
      c.name == "debug"
      msg := "不允许运行 debug 容器"
    }
```

### 违规消息格式

策略触发时，响应中包含 Policy 名称、贡献该违规的所有组以及描述：

```
[run-as-privileged via security,my-policies] container "app" must not run as privileged
  描述：Fails when securityContext.privileged is true.
```

`audit` 模式的违规以 `AdmissionResponse.Warnings` 返回（请求被放行）。

## PolicyException — 时限豁免

`PolicyException` 允许你在一定时间窗口内，针对特定命名空间或资源，绕过指定的 Policy、整个 PolicyGroup，甚至所有策略。

### 快速示例

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: PolicyException
metadata:
  name: hr-system-legacy-migration
spec:
  policyRefs:
    - run-as-privileged
  match:
    namespaces: [hr-system]
  duration: 24h
  reason: "遗留计费系统迁移；工单 OPS-1245"
```

### 字段说明

- `policyRefs` / `policyGroupRefs` / `allPolicies` — 三选一。分别对应具体 Policy、整个 PolicyGroup 或所有策略。
- `match.namespaces` — 精确命名空间名称列表（不支持通配符）。
- `match.namespaceSelector` — 匹配 Namespace 对象的 labels。
- `match.resourceSelector` — 匹配被准入对象本身的 labels。
- `duration` — 必填，Go `time.Duration` 格式（如 `24h`、`30m`）。
- `retainAfterExpiry` — 可选，过期后保留时长，默认 `0`（立即删除）。
- `reason` — 必填，非空的审计说明字符串。

### 规则

- 时间起点为 `metadata.creationTimestamp`。修改 `duration` 会重新计算 `status.expiresAt`，`status.effectiveAt` 永不变动。
- `Expired` 是终态 — 已过期的豁免无法通过修改 `duration` 复活，需新建对象续期。
- 只有 `duration`、`retainAfterExpiry`、`reason` 可变更；其他字段（目标、match）在创建后不可修改。
- 不可变性通过 `ValidatingAdmissionPolicy` (CEL) 强制执行，其列表字段比较（`policyRefs`、`policyGroupRefs`、`match.namespaces`）是 **有序敏感** 的。即使集合等价，重排元素的 patch 也会被判为违反不可变性。如果工具会在 apply 时对 JSON 数组重新排序，请配置为保留原顺序，或直接重建对象而非 patch。

## 独立策略示例

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: deny-privileged
spec:
  enforcementMode: enforce
  match:
    operations: [CREATE, UPDATE]
    resources:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
  rego: |
    package kubesentry

    deny[msg] {
      input.request.object.spec.containers[_].securityContext.privileged == true
      msg := "不允许运行特权容器"
    }
```

更多开箱即用的策略示例见 [`examples/`](examples/) 目录。

### Pod 拦截与 Deployment 拦截的区别

默认情况下策略匹配 `pods`，可以覆盖所有工作负载类型。当 apply 一个
`Deployment` 时，`kubectl apply` 会立即成功——Webhook 只有在 Deployment
控制器随后尝试创建 Pod 时才会触发，因此拒绝信息会出现在
`kubectl describe deployment` 的事件中，而不是命令行输出里。

如果希望 apply Deployment 时立即报错，需要在 match 规则中同时添加
`deployments`，**并且**为每种资源类型分别编写 Rego 规则，因为容器路径不同：

| 资源 | Rego 中的容器路径 |
|---|---|
| `pods` | `input.request.object.spec.containers[_]` |
| `deployments` | `input.request.object.spec.template.spec.containers[_]` |

同时拦截两种资源的策略示例见
[`examples/policy-no-privileged-with-deployments.yaml`](examples/policy-no-privileged-with-deployments.yaml)。

## 策略回滚

设置 `spec.rollbackTo.version`，Operator 自动从对应的 `PolicyVersion` 恢复 `spec.rego`、`spec.match` 和 `spec.enforcementMode`，并清除该字段：

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: Policy
metadata:
  name: deny-privileged
spec:
  rollbackTo:
    version: 2
```

## 安装

### 前置条件

- Kubernetes 1.28+
- Helm 3.8+（支持 OCI）
- 已配置的 `kubectl`

### 安装最新版本

```bash
helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --namespace kubesentry-system \
  --create-namespace
```

### 安装指定版本

```bash
helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --version 0.1.0 \
  --namespace kubesentry-system \
  --create-namespace
```

### Docker Hub 镜像

| 镜像 | 标签 |
|---|---|
| [`wynnhub/kubesentry-webhook`](https://hub.docker.com/r/wynnhub/kubesentry-webhook) | `latest`、`v0.1.0` |
| [`wynnhub/kubesentry-operator`](https://hub.docker.com/r/wynnhub/kubesentry-operator) | `latest`、`v0.1.0` |

两个镜像均为公开仓库，支持 `linux/amd64` 和 `linux/arm64`。

### 从源码安装

```bash
helm install kubesentry charts/kubesentry \
  --namespace kubesentry-system \
  --create-namespace
```

## 配置项

| 参数 | 默认值 | 说明 |
|---|---|---|
| `webhook.replicas` | `2` | Webhook 副本数 |
| `webhook.image.tag` | *(chart appVersion)* | 覆盖镜像 tag |
| `operator.replicas` | `1` | Operator 副本数 |
| `operator.image.tag` | *(chart appVersion)* | 覆盖镜像 tag |
| `tls.secretName` | `kubesentry-tls` | TLS Secret 名称 |
| `failurePolicy` | `Fail` | Webhook 失败策略 |
| `policy.versionHistoryLimit` | `20` | 每个策略保留的最大版本数 |
| `webhookNamespaceSelector` | 排除 `kube-system`、`kubesentry-system` | VWC 命名空间选择器 |
| `builtinNamespaceSelector` | 排除 `kube-system`、`kube-public`、`kube-node-lease`、`kubesentry-system` | 应用于所有内置组的默认命名空间选择器 |
| `builtinGroups.security.enabled` | `true` | 启用 Security 策略组 |
| `builtinGroups.efficiency.enabled` | `true` | 启用 Efficiency 策略组 |
| `builtinGroups.reliability.enabled` | `true` | 启用 Reliability 策略组 |
| `builtinGroups.<组>.namespaceSelector` | *(继承 builtinNamespaceSelector)* | 覆盖单个组的命名空间选择器 |
| `builtinGroups.<组>.selectorEnforcementMode` | — | 该组 bySelector 成员的默认执行模式 |
| `builtinPolicies.<name>.enabled` | `true` | 启用/禁用单条内置 Policy CR |
| `builtinPolicies.<name>.mode` | — | 覆盖单条策略的执行模式（`enforce`\|`audit`） |

## 开发

### 环境要求

- Go 1.26+
- Docker（用于跨平台构建）
- Helm 3.8+

### 常用命令

```bash
make test          # 运行全量测试
make build         # 本地编译 → bin/webhook, bin/operator
make lint          # go vet 静态检查
make helm-package  # lint + 打包 chart → dist/kubesentry-<version>.tgz
```

### 发布流程

```bash
# 首次需要登录 Docker Hub（推送时需要）
docker login -u wynnhub
helm registry login registry-1.docker.io -u wynnhub

# 打 tag 并发布
git tag v0.1.0
make release VERSION=v0.1.0
```

`make release` 按顺序执行：

| 步骤 | 命令 | 产物 |
|---|---|---|
| 1. 测试 | `go test ./...` | — |
| 2. 交叉编译 | 容器内 `go build` | `bin/linux-amd64/`、`bin/linux-arm64/` |
| 3. 推送镜像 | `docker buildx ... --push` | `wynnhub/kubesentry-webhook:v0.1.0` + `:latest` |
| 4. 打包 Chart | `helm package` | `dist/kubesentry-0.1.0.tgz` |
| 5. 推送 Chart | `helm push ... oci://` | `oci://registry-1.docker.io/wynnhub/kubesentry:0.1.0` |

### 多平台镜像说明

镜像以 OCI Manifest List 形式发布，Kubernetes 在拉取时根据节点架构自动选择，无需任何额外配置。

```bash
# 查看已发布的平台信息
docker buildx imagetools inspect wynnhub/kubesentry-webhook:latest
```

### 项目结构

```
kubesentry/
├── cmd/
│   ├── webhook/main.go       # Webhook 服务器入口
│   └── operator/main.go      # Operator + tls-setup 子命令
├── internal/
│   ├── api/v1alpha1/         # CRD 类型定义
│   ├── webhook/              # OPA 评估器、缓存、HTTP Handler
│   ├── operator/             # Policy、PolicyGroup 和 WebhookConfig 协调器
│   └── tlssetup/             # ECDSA 证书生成
├── charts/kubesentry/        # Helm Chart
│   ├── crds/                 # CRD 清单（Policy、PolicyVersion、PolicyGroup）
│   ├── builtin-policies/     # 内置策略目录（唯一来源）
│   │   ├── policies/         # 37 个独立 Policy CR（每条策略一个 yaml）
│   │   └── groups/           # 3 个 PolicyGroup 成员清单
│   └── templates/            # Helm 渲染模板（builtin-policies.yaml + builtin-groups.yaml）
├── test/builtins/            # helm template + CompileRego 冒烟测试
├── Dockerfile.webhook        # 仅运行时镜像，不含构建步骤
└── Dockerfile.operator
```

## 许可证

Apache License 2.0 — 详见 [LICENSE](LICENSE)。
