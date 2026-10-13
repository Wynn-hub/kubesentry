# 网络策略与端口

| 来源 | 目标 | 端口 | 方向/说明 |
|---|---|---:|---|
| 用户浏览器 | Console NodePort/Ingress | TCP 30080/自定义 | 入站；生产使用 HTTPS |
| Console Web | Console API | TCP 8081 | 同 Pod localhost |
| Console API | Gateway | TCP 9443 | 中台集群内部 |
| Operator | Gateway | TLS/TCP 30443 | 成员集群主动出站；Service 9443 |
| Console API、Gateway、DB Job | PostgreSQL | TCP 5432 | 中台集群内部 |
| kube-apiserver | Webhook Service | TCP 443 | 成员集群内部；目标 8443 |
| Webhook | Operator Events | TCP 8082 | 成员集群内部 |
| 组件 | Kubernetes API / DNS | TCP 443 / UDP,TCP 53 | 集群基础能力 |
| 节点 | 镜像仓库 | TCP 443 | 拉取镜像时出站 |

NetworkPolicy 应按上述来源 Pod selector 放行，拒绝无关命名空间直接访问 PostgreSQL、Console API、Events Server 和 Webhook 容器端口。使用 NodePort 时还要在节点防火墙或安全组开放 Console 30080（面向管理员来源）和 Gateway 30443（面向成员集群出口地址）。端口被 values 覆盖后，本文、README、帮助中心及防火墙规则必须同步更新。
