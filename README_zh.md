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
│  │  - OPA 评估器    │      │  - 创建子 Policy CRD           │  │
│  │  - 策略缓存      │      │  - 内置策略库（37 条规则）      │  │
│  │  - /validate     │      │  - 冲突检测                    │  │
│  │  - /healthz      │      │                                │  │
│  │  - /readyz       │      │  PolicyReconciler              │  │
│  └────────┬─────────┘      │  - 验证 Rego                   │  │
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
- **内置策略组** — 37 条精选规则，覆盖 Security、Efficiency、Reliability 三个分组，安装时自动部署
- **策略组管理** — 支持分组级别和单条策略的独立开关
- **自定义策略组** — 创建自己的 `PolicyGroup` CRD；当自定义策略与内置策略的 key 相同时，自定义策略优先
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

### 自定义内置组行为

通过 Helm values 关闭整个组或单条策略：

```yaml
builtinGroups:
  security:
    enabled: true
    policies:
      hostNetworkSet:
        enabled: false       # 禁用该策略
      runAsRootAllowed:
        mode: enforce        # 将模式覆盖为 enforce
  efficiency:
    enabled: false           # 禁用整个组
```

## PolicyGroup CRD

也可以创建自己的策略组，混合使用内置策略和自定义策略：

```yaml
apiVersion: kubesentry.io/v1alpha1
kind: PolicyGroup
metadata:
  name: my-policies
spec:
  enabled: true
  displayName: "我的自定义策略"
  policies:
    # 使用内置策略并覆盖模式
    - key: runAsPrivileged
      mode: enforce
    # 不在内置库中的自定义策略
    - key: noDebugContainers
      mode: enforce
      rego: |
        package kubesentry
        deny[msg] {
          c := input.request.object.spec.containers[_]
          c.name == "debug"
          msg := "不允许运行 debug 容器"
        }
      match:
        operations: [CREATE, UPDATE]
        resources:
          - apiGroups: [""]
            apiVersions: ["v1"]
            resources: ["pods"]
```

当 `PolicyGroup` 中的某个 key 与已存在的独立 `Policy`（即没有 `OwnerReference` 指向任何 `PolicyGroup` 的策略）相同时，独立策略优先，组内该条目被跳过。

### 违规消息格式

策略触发时，响应中包含所属组、key 和描述：

```
[security/runAsPrivileged] container "app" must not run as privileged
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
| `webhookNamespaceSelector` | 排除 `kube-system`、`kubesentry-system` | 命名空间选择器 |
| `builtinGroups.security.enabled` | `true` | 启用 Security 策略组 |
| `builtinGroups.efficiency.enabled` | `true` | 启用 Efficiency 策略组 |
| `builtinGroups.reliability.enabled` | `true` | 启用 Reliability 策略组 |
| `builtinGroups.<组>.policies.<key>.enabled` | — | 启用/禁用单条内置策略 |
| `builtinGroups.<组>.policies.<key>.mode` | — | 覆盖单条策略的模式（`enforce`\|`audit`） |

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
│   ├── builtins/             # 内置 Rego 策略库（37 条规则）
│   │   └── rego/             # .rego 文件（每条策略一个）
│   ├── webhook/              # OPA 评估器、缓存、HTTP Handler
│   ├── operator/             # Policy、PolicyGroup 和 WebhookConfig 协调器
│   └── tlssetup/             # ECDSA 证书生成
├── charts/kubesentry/        # Helm Chart
│   ├── crds/                 # CRD 清单（Policy、PolicyVersion、PolicyGroup）
│   └── templates/            # Kubernetes 资源模板
├── Dockerfile.webhook        # 仅运行时镜像，不含构建步骤
└── Dockerfile.operator
```

## 许可证

Apache License 2.0 — 详见 [LICENSE](LICENSE)。
