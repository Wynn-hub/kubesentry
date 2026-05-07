# KubeSentry

[English](README.md) | 中文

基于 OPA/Rego 策略引擎的 Kubernetes Validating Admission Webhook，通过 CRD 管理策略生命周期，支持版本控制与回滚。

## 架构

两个独立的 Go 二进制程序共享相同的 CRD API 类型：

```
┌─────────────────────────────────────────────────────────┐
│                    Kubernetes 集群                       │
│                                                         │
│  ┌──────────────────┐      ┌──────────────────────────┐ │
│  │  kubesentry-     │      │   kubesentry-operator    │ │
│  │  webhook         │      │                          │ │
│  │                  │      │  PolicyReconciler        │ │
│  │  - OPA 评估器    │      │  - 验证 Rego             │ │
│  │  - 策略缓存      │      │  - 创建 PolicyVersion    │ │
│  │  - /validate     │      │  - 处理回滚              │ │
│  │  - /healthz      │      │                          │ │
│  │  - /readyz       │      │  WebhookConfigReconciler │ │
│  └────────┬─────────┘      │  - 聚合规则              │ │
│           │                │  - 更新 VWC              │ │
│           │ 监听           └──────────────────────────┘ │
│           ▼                                             │
│  ┌──────────────────┐      ┌──────────────────────────┐ │
│  │   Policy CRD     │      │  PolicyVersion CRD       │ │
│  │   （集群级别）    │      │  （不可变快照）           │ │
│  └──────────────────┘      └──────────────────────────┘ │
└─────────────────────────────────────────────────────────┘
```

## 功能特性

- **嵌入式 OPA 引擎** — 直接内嵌 OPA，无需 sidecar
- **CRD 策略管理** — 以 `Policy` Kubernetes 资源定义策略
- **双执行模式** — `enforce`（拦截）或 `audit`（仅记录日志）
- **版本控制** — 每次策略变更自动创建不可变的 `PolicyVersion` 快照
- **一键回滚** — 设置 `spec.rollbackTo.version` 即可还原任意历史版本
- **动态规则同步** — Operator 自动根据 Ready 状态的策略更新 `ValidatingWebhookConfiguration`
- **自签 TLS** — Helm 安装时通过 pre-install Job 自动生成证书并注入 `caBundle`
- **并发评估** — 多条策略并发执行，超时时间 5 秒
- **Leader 选举** — Operator 支持高可用 Leader 选举
- **多平台镜像** — 同时支持 `linux/amd64` 和 `linux/arm64`

## 策略示例

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

### 从 Docker Hub 安装

```bash
# 首次登录（私有仓库需要）
helm registry login registry-1.docker.io -u wynnhub

helm install kubesentry \
  oci://registry-1.docker.io/wynnhub/kubesentry \
  --version 0.1.0 \
  --namespace kubesentry-system \
  --create-namespace
```

Helm pre-install Job 会自动生成自签 CA 和服务端证书，存入 Secret，并将 `caBundle` 注入 `ValidatingWebhookConfiguration`。

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
| `operator.replicas` | `1` | Operator 副本数 |
| `tls.secretName` | `kubesentry-tls` | TLS Secret 名称 |
| `failurePolicy` | `Fail` | Webhook 失败策略 |
| `policy.versionHistoryLimit` | `20` | 每个策略保留的最大版本数 |
| `webhookNamespaceSelector` | 排除 `kube-system`、`kubesentry-system` | 命名空间选择器 |

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
# 首次需要登录
docker login -u wynnhub
helm registry login registry-1.docker.io -u wynnhub

# 打 tag 并发布
git tag v0.1.0
make release
```

`make release` 按顺序执行：

| 步骤 | 命令 | 产物 |
|---|---|---|
| 1. 测试 | `go test ./...` | — |
| 2. 交叉编译 | 容器内 `go build` | `bin/linux-amd64/`、`bin/linux-arm64/` |
| 3. 推送镜像 | `docker buildx ... --push` | `wynnhub/kubesentry-webhook:v0.1.0`（amd64 + arm64 manifest） |
| 4. 打包 Chart | `helm package` | `dist/kubesentry-0.1.0.tgz` |
| 5. 推送 Chart | `helm push ... oci://` | `oci://registry-1.docker.io/wynnhub/kubesentry:0.1.0` |

### 多平台镜像说明

镜像以 OCI Manifest List 形式发布，Kubernetes 在拉取时根据节点架构自动选择，无需任何额外配置。

```bash
# 查看已发布的平台信息
docker buildx imagetools inspect wynnhub/kubesentry-webhook:v0.1.0
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
│   ├── operator/             # Policy 和 WebhookConfig 协调器
│   └── tlssetup/             # ECDSA 证书生成
├── charts/kubesentry/        # Helm Chart
│   ├── crds/                 # CRD 清单
│   └── templates/            # Kubernetes 资源模板
├── Dockerfile.webhook        # 仅运行时镜像，不含构建步骤
└── Dockerfile.operator
```

## 许可证

Apache License 2.0 — 详见 [LICENSE](LICENSE)。
