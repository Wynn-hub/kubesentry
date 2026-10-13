# 安装与升级

## 中台

```bash
helm install kubesentry-gateway ./charts/kubesentry-gateway -n kubesentry-system --create-namespace
kubectl get pods -n kubesentry-system
```

访问 `http://<NodeIP>:30080`，为默认 `admin` 设置密码。生产环境应配置 HTTPS、`console.session.cookieSecure=true`，并替换默认 PostgreSQL 密码或使用外部数据库。

## 成员集群

在控制台创建集群，按页面选择高可用等参数，复制完整 Helm 命令到目标集群执行。不要记录或复用一次性注册令牌。确认集群状态变为“在线”，再分配给租户。

## 升级与卸载

使用 `helm upgrade` 更新中台或成员 Chart。升级前备份 PostgreSQL。成员集群应从控制台发起卸载，使 Operator 清理托管资源；完成后再删除集群记录。回滚使用上一版本 Chart/镜像和数据库备份，数据库迁移是否可逆须在变更说明中确认。
