# 集群在线自卸载与策略直接配置作用范围 Implementation Plan

> **Prerequisite:** 实施前以 `docs/superpowers/specs/2026-07-22-cluster-uninstall-and-policy-scope-design.md` 为准。此计划将“在线自卸载”和“策略范围直配”拆为可独立验证的提交单元；所有提交都须在用户明确确认后执行。

**Goal:** 让在线成员集群接收持久化的自卸载任务并清理 KubeSentry 自有资源（保留命名空间），同时将策略组、豁免策略的目标集群配置收敛到其创建/编辑页，保留 `PolicyDistribution` 为不可见的内部渲染实现。

**Architecture:** PostgreSQL 保存集群操作任务和内部下发记录；Gateway 将待处理任务及匹配的策略/豁免快照通过现有 gRPC `Sync` 流发送给目标 Operator。成员端使用最小权限 Kubernetes API 清理 allowlist 中的资源，并回传任务状态。console-api 在同一事务中维护业务对象与其内部下发记录，前端不再展示 Distribution 页面。

**Tech Stack:** Go、GORM/PostgreSQL migrations、gRPC/protobuf、controller-runtime client、Helm、Vue 3/TypeScript、Element Plus。

## Global constraints

- Go 命令使用 `GOROOT=/opt/homebrew/opt/go/libexec`；不运行 `make generate`。
- 所有 protobuf 生成文件提交入库；协议变更必须保持旧成员端的安全降级。
- 自卸载绝不删除命名空间，绝不接受平台提供的任意资源名或 shell 命令。
- 资源清理使用明确名称与 KubeSentry release 标签的交集；`NotFound` 是成功。
- `default` 策略组范围恒为全部集群且不可修改；非 default 的策略组和所有豁免策略范围不能为空。
- 不自动提交、推送或部署。每个 commit 都先取得用户对语言、消息和提交动作的确认。

## Task 1: 数据模型、迁移与仓储基础

**Files:**
- Modify: `internal/dbrepo/models.go`, `internal/dbrepo/cluster_repo.go`, `internal/dbrepo/distribution_repo.go`
- Create: `internal/dbrepo/uninstall_task_repo.go`, `internal/dbmigrate/migrations/000x_cluster_uninstall_and_scopes.up.sql`, `internal/dbmigrate/migrations/000x_cluster_uninstall_and_scopes.down.sql`
- Test: `internal/dbrepo/*_test.go`, `internal/dbmigrate/migrate_test.go`

- [ ] 定义 `UninstallTask`、任务状态和集群 `Uninstalling`/`Uninstalled` phase；增加按集群查找活动任务、原子状态转换、重试计数和最终结果写入的 repo 方法。
- [ ] 为内部下发记录增加 owner type/name、managed 标识和豁免引用；保证一条业务对象恰有一条托管记录。
- [ ] 编写 migration：创建任务表、索引和约束；为 default 建立固定全量内部记录；为可唯一映射的历史 Distribution 回填 owner，保留无法无损迁移的原始记录和诊断信息。
- [ ] 先写 repo/migration 测试，覆盖并发状态转换、重试、唯一 owner 约束、default 固定范围和迁移回滚。

**Verify:** `GOROOT=/opt/homebrew/opt/go/libexec go test ./internal/dbrepo/... ./internal/dbmigrate/...`

## Task 2: 扩展同步协议与 Gateway 任务投递

**Files:**
- Modify: `api/proto/gateway/v1/gateway.proto`, `Makefile`
- Regenerate: `internal/gatewaypb/*.pb.go`
- Modify: `internal/gateway/server.go`, `internal/gateway/render.go`, `internal/gateway/registry.go`
- Test: `internal/gateway/*_test.go`

- [ ] 在 `DesiredState` 中增加版本化的卸载任务命令；在 `MemberMessage` 中增加任务接收、成功、失败回报。保留现有字段号码，新增字段只追加。
- [ ] Gateway 从持久化任务查询目标集群待执行操作，并把任务与策略快照一同渲染到该集群的 Sync 流。
- [ ] 处理 member 回报时，使用 compare-and-swap 验证任务/集群归属和允许的状态流转；成功时更新集群为 `Uninstalled`，失败时为 `UninstallFailed`。
- [ ] 对不支持新字段的成员端保持安全：不将集群误标为完成；console 显示不支持/超时，而不是乐观成功。

**Verify:** `make proto`; `GOROOT=/opt/homebrew/opt/go/libexec go test ./internal/gateway/...`

## Task 3: 成员端卸载执行器与最小 RBAC

**Files:**
- Create: `internal/operator/uninstall/runner.go`, `internal/operator/uninstall/runner_test.go`
- Modify: `internal/operator/hublink/client.go`, `internal/operator/hublink/apply.go`, `cmd/operator/main.go`
- Modify: `charts/kubesentry/templates/operator/rbac.yaml`, `charts/kubesentry/templates/operator/deployment.yaml`, `charts/kubesentry/values.yaml`
- Test: `internal/operator/hublink/*_test.go`, chart rendering assertions as appropriate

- [ ] 从 desired state 提取目标为本集群的卸载任务；先发送 received 回报，再启动幂等 Runner。
- [ ] 用 fake client 驱动 Runner 的 allowlist 清理：VWC、KubeSentry CR、工作负载/服务/TLS Secret、RBAC/SA、CRD、最后 Operator 自身。
- [ ] 对每类资源使用确定名称加 release 标签验证；禁止 Namespace delete；记录任务 ID 与资源结果但不记录令牌。
- [ ] 设计“最后身份”顺序：完成回报在删除自身运行资源前发送；剩余资源在可重试操作中清理，确保不会因已删除对象而报错。
- [ ] Helm RBAC 只增加 Runner 所需的具体资源和动词，使用 `resourceNames` 的地方保持限定；渲染后人工审查 delete 权限。

**Verify:** `GOROOT=/opt/homebrew/opt/go/libexec go test ./internal/operator/...`; `helm lint charts/kubesentry`; `helm template kubesentry charts/kubesentry --namespace kubesentry-system`。

## Task 4: console-api 卸载任务接口

**Files:**
- Modify: `internal/console/handlers_cluster.go`, `internal/console/server.go`, `internal/console/response.go` as needed
- Create/Modify: `internal/console/handlers_cluster_uninstall_test.go`, `internal/console/testhelper_db_test.go`

- [ ] 实现创建、列表/详情和重试接口；只允许 Online 集群创建，拒绝并发活动任务和非法状态。
- [ ] 创建任务后设置 `Uninstalling` 并调用 `NotifyChange`；失败/重试也必须通知 Gateway。
- [ ] 保持原删除规则：`Uninstalled` 记录可以由现有删除操作永久移除，卸载成功本身不删除记录。
- [ ] 测试二次请求冲突、Offline 拒绝、成功/失败回报后的可见状态、重试、审计字段和分页。

**Verify:** `GOROOT=/opt/homebrew/opt/go/libexec go test ./internal/console/...`

## Task 5: 策略组与豁免策略的范围服务

**Files:**
- Modify: `internal/console/handlers_group.go`, `internal/console/handlers_exception.go`, `internal/console/handlers_distribution.go`, `internal/console/server.go`
- Create: `internal/console/scope_binding.go`, `internal/console/scope_binding_test.go`
- Modify: `internal/gateway/render.go`, `internal/gateway/render_test.go`

- [ ] 定义统一范围请求：`allClusters` 或非空 `clusterNames`，两者互斥；查询集群存在性并排除 `Uninstalled`。
- [ ] 创建/更新策略组或豁免策略时，在 DB transaction 内创建/更新对应内部 `PolicyDistribution`，失败时整体回滚。
- [ ] default 策略组只允许固定全量范围；API 对范围修改返回明确错误，启停接口继续可用。
- [ ] Gateway 渲染内部记录时，同时收集 PolicyGroup 与 PolicyException 引用，匹配“全部集群”以及指定集群；保持原有策略模式冲突处理。
- [ ] 移除公开 Distribution handler/router，只保留内部 repo 与渲染使用；删除行为由删除组/豁免策略时一并删除所属内部记录完成。

**Verify:** `GOROOT=/opt/homebrew/opt/go/libexec go test ./internal/console/... ./internal/gateway/...`

## Task 6: 前端交互与 i18n

**Files:**
- Modify: `web/src/api/cluster.ts`, `web/src/api/group.ts`, `web/src/api/exception.ts`, `web/src/router/index.ts`, `web/src/layouts/AppLayout.vue`
- Modify: `web/src/views/ClusterDetailView.vue`, `web/src/views/ClusterListView.vue`, `web/src/views/GroupFormView.vue`, `web/src/views/ExceptionFormView.vue`, corresponding list/detail views
- Delete: `web/src/api/distribution.ts`, `web/src/views/DistributionListView.vue`, `web/src/views/DistributionFormView.vue` and their tests only after routes/imports are removed
- Modify: `web/src/i18n/zh-CN.ts`, `web/src/i18n/en-US.ts`

- [ ] 在集群详情中实现危险卸载弹窗：说明保留命名空间、要求输入准确集群名、提交后轮询/刷新任务状态、失败显示重试。
- [ ] 在策略组和豁免策略表单实现“全部集群”开关和集群多选；客户端预校验范围必填，服务端错误原样可读展示。
- [ ] default 策略组显示只读的“全部集群（内置）”，不渲染范围编辑器。
- [ ] 列表/详情显示范围摘要；移除策略下发入口、路由、i18n 和遗留 API 调用。
- [ ] 为纯 API/状态转换逻辑补齐现有 Vitest 测试模式。

**Verify:** `cd web && npm test && npm run typecheck && npm run build`（按项目实际 script 名称调整）。

## Task 7: 端到端验证、迁移演练与文档

**Files:**
- Modify: `test/e2e/t11_distribution_test.go` or successor tests, `test/e2e/fixtures/`
- Create: targeted uninstall e2e coverage and migration fixture as existing harness conventions require
- Modify: chart README/console usage documentation if current user instructions mention Distribution

- [ ] 在测试环境创建在线成员、创建范围为全部/指定集群的策略组和豁免策略，验证现有和新上线集群获得正确快照。
- [ ] 发起卸载后验证任务回报、允许资源已清理、命名空间仍存在、集群记录变为 `Uninstalled`；重复任务不误删其他资源。
- [ ] 演练包含 default 和多对象历史 Distribution 的升级迁移，检查无损对象收敛与异常诊断。
- [ ] 执行 `make test`、`make lint`、Helm lint/template、前端检查、相关 e2e；检查 diff、无调试残留、无明文 token。

## Commit checkpoints

建议按下列边界准备提交，但每次均等待用户明确确认：

1. `feat: add persistent cluster uninstall tasks`
2. `feat: deliver member cluster uninstall commands`
3. `feat: configure policy scopes from policy forms`
4. `feat: update console uninstall and scope workflows`
5. `test: cover cluster uninstall and policy scope flows`

