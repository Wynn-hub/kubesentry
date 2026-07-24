# 简介

## 目的

将 Gateway 从“多个副本均提供 gRPC 服务、但实时状态保存在各自内存中”的模型调整为 Kubernetes 主备模型。任意时刻仅一个 Gateway leader 对外提供 gRPC 服务，避免跨副本的 `Sync` 会话、通知器、心跳缓存及 `Dispatch` 路由不一致。

## 术语定义

| 简称/术语 | 说明 |
| --------- | ---- |
| Gateway leader | 持有 `kubesentry-gateway-leader` Lease 的 Gateway Pod。 |
| Ready Endpoint | 通过 `/readyz` 检查、被纳入 Gateway Service EndpointSlice 的 Pod。 |
| 主备切换 | leader 失去 Lease 后停止服务，由新 Lease 持有者恢复服务的过程。 |

## 参考资料

| 序号 | 文档名称 | 最后修订时间 | 版本号 | 来源 | 作者 |
| ---- | -------- | ------------ | ------ | ---- | ---- |
| 1 | Gateway 数据库架构设计 | 2026-07-16 | 当前 | `docs/superpowers/specs/2026-07-16-hub-database-design.md` | 项目组 |
| 2 | Kubernetes Lease leader election | 2026-07-24 | Kubernetes v1.31 | Kubernetes client-go | 项目组 |

# 系统概述

## 系统目标

Gateway 支持多副本部署与故障切换，但同一时刻仅一个副本接受 operator/console 的 gRPC 连接、刷新集群状态并维护实时内存状态。NodePort 从任意节点进入的流量均能转发给当前 leader。

## 设计约束

- 业务数据继续保存在共享 PostgreSQL；不新增 Redis、消息队列或数据库迁移。
- Gateway Service 使用 NodePort/LoadBalancer 时必须保留 `externalTrafficPolicy: Cluster`，不得使用 `Local`。
- 所有 Gateway Pod 均需要获取和更新同命名空间的 Lease 的最小 RBAC 权限。
- 既有 operator 与 console 的重连机制承担切换期间长连接重建。

## 设计思路

使用 `coordination.k8s.io/v1 Lease` 选出 leader。健康探针始终存活，但 readiness 仅在 leader 的 gRPC listener 已启动时成功。Kubernetes 会据此仅将 leader 加入 Service EndpointSlice；默认的 NodePort `Cluster` 转发从任意节点将新连接送至该 Endpoint。

## 系统总体架构

```text
operator / console
       |
任意 NodeIP:NodePort
       |
Gateway Service (EndpointSlice: only Ready leader)
       |
Gateway leader Pod -- gRPC Sync / Notifier / StatusFlusher
       |
 PostgreSQL

Gateway standby Pods -- /healthz only, /readyz = 503
```

# 功能设计

## 模块设计

| 模块 | 设计 |
| ---- | ---- |
| Gateway leader election | 全部副本竞争 `kubesentry-gateway-leader` Lease，使用 Pod 名称作为 identity。 |
| gRPC 服务 | 仅 leader 创建 TLS 证书、启动 gRPC listener 与 Hub 服务。 |
| 状态刷新 | 仅 leader 启动 `StatusFlusher`，防止 standby 因无本地心跳将在线集群误置为离线。 |
| 健康检查 | `/healthz` 表示进程存活；`/readyz` 仅当 leader gRPC 已可服务时返回 200。 |
| Service 路由 | NodePort/LoadBalancer 设置 `externalTrafficPolicy: Cluster`；非 leader 不在 EndpointSlice 内。 |

## 模块设计详细描述

leader 失去 Lease 时，先将 readiness 置为 false，再优雅停止 gRPC 服务并取消状态刷写。新 leader 获取 Lease 后创建或读取 TLS Secret、启动 gRPC 和状态刷写，最后将 readiness 置为 true。operator 和 console 的现有指数退避机制重建断开的流。

## 接口设计（可选）

对外 gRPC API、REST API 和 Helm Values 不新增业务接口。`/healthz` 与 `/readyz` 保持现有路径与语义，其中 `/readyz` 的语义从“本 Pod gRPC 已启动”收紧为“本 Pod 是可服务 leader”。

# 非功能设计

## 可靠性

Lease 使用 15 秒租约、10 秒续约截止和 2 秒重试周期。正常优雅关闭主动释放 Lease；非正常失联后，切换延迟受租约周期与 EndpointSlice 收敛时间共同影响。

## 安全性

Gateway ServiceAccount 对 Lease 仅授予 `get/list/watch/create/update/patch`。TLS 私钥仍保存于 Kubernetes Secret，不新增明文配置。

## 升级/补丁

滚动升级期间旧 leader 变为 NotReady 后，新副本接管 Lease。短暂断开的 gRPC 长连接由客户端自动重连；策略与集群记录保持在 PostgreSQL 中，不会重新注册集群。

## 安装部署

通过 `gateway.replicas` 设置副本数。NodePort 场景访问任意节点 IP 加 NodePort；集群内组件继续使用 Service DNS。不得为该 Service 设置 `externalTrafficPolicy: Local`。
