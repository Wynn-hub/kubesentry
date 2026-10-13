# 架构与组件

KubeSentry 分为中台与成员集群两个平面。中台的 Console Web/API、Gateway、PostgreSQL 和迁移任务保存身份、租户、策略意图及事件；成员集群的 Operator、Webhook、Events Server 和 CRD 执行准入控制。

![部署架构](../diagrams/deployment-architecture.svg)

![组件数据流](../diagrams/component-architecture.svg)

控制流由 Gateway 通过 Operator 主动建立的 TLS gRPC 流下发；状态、心跳、事件和观测数据沿同一连接上报。API Server 通过 Service 443 调用 Webhook，Webhook 从本地 informer 缓存读取已编译策略，不在准入链路同步查询中台或数据库。

系统管理员不绑定租户；租户管理员和普通用户恰好绑定一个租户。服务端中间件和业务服务是权限边界。
