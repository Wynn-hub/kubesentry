<p align="center"><img src="web/public/favicon.svg" width="96" alt="KubeSentry"><br><strong>KubeSentry</strong></p>
<p align="center">面向多集群与多租户的 Kubernetes 准入安全控制平台</p>
<p align="center"><a href="README.md">English</a> · 中文</p>

[![Go](https://img.shields.io/badge/Go-1.26+-00ADD8)](https://go.dev/) [![Kubernetes](https://img.shields.io/badge/Kubernetes-1.28+-326CE5)](https://kubernetes.io/) [![License](https://img.shields.io/badge/License-Apache--2.0-green)](LICENSE)

KubeSentry 使用 OPA/Rego 在 Kubernetes 准入阶段执行安全、可靠性和资源效率策略。中台集中管理用户、租户、集群、策略下发、准入事件和平台观测，成员集群仅运行轻量的 Operator 与 Webhook。

## 核心能力

- 多集群注册、状态同步、租户分配和安全卸载。
- 系统管理员、租户管理员、普通用户三级权限，接口层强制鉴权。
- 全局共享策略、租户策略组、豁免、audit/enforce 模式和版本回滚。
- 37 条内置规则；准入事件检索、存储设置和平台组件观测。
- 中文/英文控制台、API 文档和登录后帮助中心。

## 部署架构

![KubeSentry 部署架构](docs/diagrams/deployment-architecture.svg)

| 区域 | 组件 | 作用 |
|---|---|---|
| 中台 | Console Web / API | 管理界面、身份认证、授权与 REST API |
| 中台 | Gateway | 集群注册、双向同步、策略下发和事件接收 |
| 中台 | PostgreSQL / DB Job | 数据持久化和迁移 |
| 成员集群 | Operator | 连接 Gateway、应用期望状态、维护 CRD 和上报状态 |
| 成员集群 | Webhook / Events | 执行准入策略并采集判定事件 |

## 组件与数据流

![KubeSentry 组件数据流](docs/diagrams/component-architecture.svg)

成员集群 Operator 主动建立到 Gateway 的 TLS gRPC 长连接，中台无需主动访问成员集群。详见[架构文档](docs/zh-CN/architecture.md)。

## 快速安装

前置条件：Kubernetes 1.28+、Helm 3.8+、默认 StorageClass，且成员集群能访问中台 Gateway。

```bash
helm install kubesentry-gateway ./charts/kubesentry-gateway \
  --namespace kubesentry-system --create-namespace
```

访问 `http://<NodeIP>:30080`，首次使用默认用户名 `admin` 设置密码。生产环境请使用 HTTPS 并设置 `console.session.cookieSecure=true`。然后在“集群管理”中新建集群，在目标集群执行页面生成的一次性 Helm 命令。详见[安装指南](docs/zh-CN/installation.md)。

## 网络要求

| 来源 | 目标 | 默认端口 | 用途 |
|---|---|---:|---|
| 浏览器 | Console | TCP 30080（Service 8080） | Web 管理界面 |
| 成员 Operator | Gateway | TLS/TCP 30443（Service 9443） | 注册、同步、上报 |
| Console API | Gateway | TCP 9443 | 平台通信 |
| 平台组件 | PostgreSQL | TCP 5432 | 数据访问 |
| kube-apiserver | Webhook | TCP 443（容器 8443） | AdmissionReview |
| Webhook | Operator Events | TCP 8082 | 事件和观测上报 |

组件还需 Kubernetes API TCP 443、DNS UDP/TCP 53；拉取镜像需出站 HTTPS TCP 443。详见[网络策略与端口](docs/zh-CN/network.md)。

## 安全规则来源与设计参考

安全、可靠性和资源效率规则参考 [Fairwinds Polaris](https://github.com/FairwindsOps/polaris) 的公开实践，并结合多集群、租户权限和 Rego 模型独立实现。该说明是总体设计参考，不表示规则逐条兼容或同步。建议先使用 `audit` 评估影响，再逐步启用 `enforce`。

## 文档

- [安装与升级](docs/zh-CN/installation.md) · [架构与组件](docs/zh-CN/architecture.md)
- [网络策略与端口](docs/zh-CN/network.md) · [文档维护指南](docs/zh-CN/documentation-maintenance.md)
- 登录控制台后访问“系统 → 帮助中心”。

## 开发

```bash
make test && make build
helm lint charts/kubesentry-gateway charts/kubesentry
cd web && npm test && npm run build
```

功能、权限、API、组件、端口或 Helm 参数变更必须按[维护指南](docs/zh-CN/documentation-maintenance.md)同步中英文文档和帮助中心。

## License

[Apache License 2.0](LICENSE)
