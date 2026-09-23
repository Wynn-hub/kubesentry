# KubeSentry 项目边界治理与认证就绪设计

## 1. 引言

### 1.1 编写目的

本文定义 KubeSentry 在 Console Swag code-first 迁移完成后的下一阶段结构治理方案。目标不是套用某种标准目录模板，而是解决已经存在的导航、归属和依赖方向问题，并为未来 Console 认证留下最小且真实的接入边界。

本文是后续实施计划的设计依据，不授权直接修改代码、生成配置、数据库迁移、部署资源或 Git 历史。

### 1.2 项目背景

仓库当前包含 `console`、`gateway`、`operator`、`webhook`、`db` 五个 Go 二进制以及三类独立契约：

- Console REST：Go handler/DTO 上的 Swag 注释是唯一事实来源；`api/swagger/swagger.yaml` 是生成的 Swagger 2 产物，`internal/console/openapi_bundle.json` 是可重复派生的 OAS 3.0 产物。
- Gateway：`api/proto/gateway/v1/gateway.proto` 是唯一协议源，对外只提供 gRPC。
- Member Cluster：CRD schema 与对应 Go 类型是 Kubernetes 公共契约。

Swag 迁移已经完成。当前问题主要是：公共 CRD Go 类型仍位于 `internal`；数据库、admission、platform 等领域包平铺在顶层 `internal`；少数跨 plane 依赖需要继续收紧；Console 尚未形成明确的未来认证接入位置。

### 1.3 术语与缩略语

| 术语 | 含义 |
| --- | --- |
| member plane | 运行 operator、webhook 并持有成员 CRD 的成员集群侧 |
| gateway plane | 运行 gateway、console、db 并持有中心数据库的管理侧 |
| 公共 Go API | 允许仓库外 Go 模块导入，并承担版本兼容责任的包 |
| 认证就绪 | 明确认证中间件插入点和路由分类，但不实现具体认证协议 |

### 1.4 参考资料

- `docs/superpowers/specs/2026-09-07-project-layout-refactor-analysis.md`
- `docs/superpowers/plans/2026-09-07-project-layout-refactor-plan.md`
- `docs/superpowers/specs/2026-09-07-console-swag-code-first-migration-design.md`
- `docs/superpowers/specs/2026-08-03-api-contract-standardization-design.md`
- `CLAUDE.md`

## 2. 系统概述

### 2.1 建设目标

1. 将成员 CRD Go 类型迁移到 `api/kubesentry/v1alpha1`，正式提供仓库外可导入的公共 Go API。
2. 将数据库、admission 和 platform 包按真实领域归组，降低顶层 `internal` 的认知负担。
3. 保持 Gateway gRPC 协议、服务实现、认证附属协议和内部模型的边界，消除跨 plane 实现依赖。
4. 稳定 Console 的 DTO、handler、service、repository 方向，并建立不改变当前访问行为的认证接入位置。
5. 通过轻量依赖门禁防止目录治理完成后再次退化。

### 2.2 非目标

- 不拆分五个 `cmd`，不引入统一进程框架。
- 不为 Gateway 增加 REST、HTTP OpenAPI 或第二套对外协议。
- 不实现 OIDC、JWT、反向代理认证、RBAC 或租户模型。
- 不建立 `pkg`、`common`、`shared`、`utils`、公共 logging SDK 或插件框架。
- 不抽取 `internal/testutil`，不为消除少量测试重复扩大生产 API。
- 不修改数据库 schema、迁移历史、protobuf field/RPC、CRD group/version 或现有 REST 行为。
- 不重新实施已经完成的 Swag 迁移。

### 2.3 设计原则

- 公共性由仓库外消费者和兼容承诺决定，不由调用方数量决定。
- 协议共享不等于实现共享；operator/hublink 可以依赖 Gateway protobuf，但不能依赖 Gateway 服务实现。
- 每个目录必须对应可解释的领域或组件，不为视觉对称增加空壳层。
- 目录移动与行为修改分离，优先使用机械迁移和窄验证。
- 认证只预留实际插入点；在身份源确定前不创建 provider、factory 或单实现 interface。

## 3. 总体架构设计

### 3.1 目标依赖关系

~~~text
公共/跨语言契约
  Console REST: handler/DTO Swag annotations
  Gateway:      api/proto/gateway/v1 -> internal/gatewaypb (generated)
  Member CRD:   api/kubesentry/v1alpha1

进程装配
  cmd/console  -> internal/console  -> internal/database/*
  cmd/gateway  -> internal/gateway  -> gatewaypb/gatewaymodel/database/*
  cmd/operator -> internal/operator -> public CRD API + gatewaypb/gatewayauth
  cmd/webhook  -> internal/webhook  -> public CRD API + admission/platform
  cmd/db       -> internal/database/migrate
~~~

必须遵守以下方向：

- `api/kubesentry/v1alpha1` 不得导入任何 `internal/*` 包。
- `cmd/*` 可以装配具体实现，领域包不得反向依赖 `cmd/*`。
- operator/hublink 不得导入 `internal/gateway` 或 `internal/database/repository`。
- Console/Gateway 可以使用公共 CRD 的 schema/spec 类型，但不得因此持有 Kubernetes client、Scheme 或 reconciler 生命周期。
- `internal/gatewaypb` 是生成实现，不因 protobuf 属于协议就自动成为公共 SDK。

### 3.2 目标目录结构

~~~text
api/
├── kubesentry/
│   └── v1alpha1/                 # 公共成员 CRD Go API
├── proto/gateway/v1/             # Gateway gRPC 唯一协议源
├── swagger/                      # Console Swagger 2 生成产物
└── openapi.yaml                  # 三类契约文档索引

internal/
├── console/                      # Console REST 实现
│   ├── dto/
│   ├── handler/
│   ├── repository/
│   └── service/
├── gateway/                      # Gateway gRPC 服务实现
├── gatewayauth/                  # Gateway gRPC 认证附属协议
├── gatewaymodel/                 # Gateway plane 内部模型
├── gatewaypb/                    # protobuf 生成代码
├── database/
│   ├── migrate/
│   ├── model/
│   └── repository/
├── admission/
│   ├── observability/
│   └── policy/
├── platform/
│   └── observability/
├── groupresolve/
├── logging/
└── tlssetup/
~~~

`groupresolve`、`logging` 和 `tlssetup` 保持顶层单一领域包。再增加一层目录不会改善依赖方向，只会增加跳转。

### 3.3 公共 CRD Go API

现有 `internal/api/v1alpha1` 中只保留了成员 CRD 类型、Scheme 注册、managed metadata 和 DeepCopy。它应整体迁移为：

~~~text
api/kubesentry/v1alpha1
~~~

兼容承诺：

- Go import path 固定为 `github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1`。
- 保持 API group、version、Kind、JSON tag、Kubebuilder marker 和 Scheme 注册行为。
- `v1alpha1` 允许增加向后兼容字段，但删除/重命名字段或改变语义必须通过新 API 版本演进。
- 后续稳定过程使用 `v1alpha1 -> v1beta1 -> v1`，旧版本是否弃用必须有转换、迁移和发布说明。
- 公共包不得暴露数据库模型、Gateway transport model 或组件实现。

旧包位于 Go `internal` 路径，仓库外本就不能合法导入，因此不保留转发兼容包。迁移必须在同一逻辑变更中更新仓库内全部调用方。

### 3.4 数据库领域

采用以下机械归组：

~~~text
internal/dbmodel   -> internal/database/model
internal/dbrepo    -> internal/database/repository
internal/dbmigrate -> internal/database/migrate
~~~

三个子包继续分开：model 提供内部持久化基础类型；repository 提供运行期数据访问；migrate 管理 schema 生命周期。此次只改变目录和 import，不修改 SQL、事务、表结构或 repository API。

它们属于 gateway plane 的仓库内基础设施，但由 console、gateway 和 db 三个二进制共同消费，因此不归入任一单独组件，也不放入 `pkg`。

### 3.5 Admission 与 Platform 领域

采用以下机械归组：

~~~text
internal/admissionobs    -> internal/admission/observability
internal/admissionpolicy -> internal/admission/policy
internal/platformobs     -> internal/platform/observability
~~~

不创建 `internal/admission` 或 `internal/platform` 的聚合 Go package，不合并现有职责。归组只改善发现性和领域导航。

### 3.6 Gateway 边界

以下四个包继续并列：

- `internal/gateway`：gRPC 服务实现、session、dispatch、render 和清理流程。
- `internal/gatewaypb`：由 protobuf 生成的 Go 类型/stub 及紧邻 wire contract 的 metadata/topic 常量。
- `internal/gatewayauth`：Gateway gRPC 组件认证 token 规则。
- `internal/gatewaymodel`：gateway plane 内部 Cluster、PolicyDistribution 等模型。

不将它们全部嵌套到 `internal/gateway/*`，因为那会把协议/认证包在目录语义上变成服务实现的私有子包，并模糊 operator/hublink 的合法依赖。

只有存在真实反向依赖时才调整 API。例如 render 若只需要 CRD Spec，就应缩窄为 Spec/明确输入；不能为了目录整洁一次性重写全部签名。

### 3.7 Console 边界与认证就绪

Console 保持：

~~~text
HTTP/router -> handler -> service -> repository -> database/repository
             DTO       use-case      adapter
~~~

规则：

- REST 请求/响应 DTO 固定在 `internal/console/dto`，继续承载 Swag schema 注释。
- handler 负责 HTTP 参数、DTO 转换和响应，不直接操作数据库。
- service 负责用例；只有真实业务需要时才接收操作者身份。
- repository 是 Console 所需的窄数据访问边界，不暴露 Gateway server 装配类型。
- 根目录保留 server、router、docs、startup 等组件装配；可测试业务规则不得继续堆入根目录。
- 纯转发且不提供隔离、事务或测试价值的薄层可以删除，但不得为追求层数机械合并有效接口。

认证就绪采用最小设计：

- 路由在装配时明确区分 `public`、`protected`、`operational`。
- 本轮不改变匿名访问行为，不实现虚假认证中间件。
- `/healthz`、`/readyz` 等 operational endpoint 保持公开。
- 在身份源确定前不创建 `Authenticator`、provider factory、RBAC engine 或插件注册表。
- 未来身份通过 `context.Context` 传播，不能进入 DTO、数据库模型或全局变量。
- Swag 可以定义 `BearerAuth`，但认证落地前 operation 不得声明强制 security。

后续认证实施顺序应为：确定身份源与信任边界；实现中间件；写入最小 principal；为具体 operation 增加授权；同步 Swag security；补充 401/403、审计和端到端测试。

### 3.8 扩展性边界

本设计支持以下真实扩展，而不提前实现：

- 新增 CRD 版本：在 `api/kubesentry/<version>` 增加版本包和转换策略。
- 新增 Gateway RPC：只修改 proto 并重新生成 `gatewaypb`，不引入 REST 镜像接口。
- 新增 Console operation：在现有 handler/service/repository 方向内增加，并通过 Swag 生成链路发布。
- 新增认证：在 Console 路由组接入中间件；具体身份源决定后再设计最小接口。
- 新增领域能力：只有被多个组件实际复用且边界稳定时才建立新的顶层 `internal/<domain>`。

## 4. 接口与契约设计

### 4.1 三类契约边界

| 契约 | 唯一事实来源 | 生成/派生产物 | 兼容门禁 |
| --- | --- | --- | --- |
| Console REST | handler/DTO Swag 注释 | Swagger 2、OAS 3.0 bundle | operation、schema、status、前端类型 |
| Gateway gRPC | gateway.proto | gatewaypb Go 代码 | descriptor、RPC、field number |
| Member CRD | 公共 Go API/Kubebuilder marker | CRD YAML、DeepCopy | group/version/kind、OpenAPI schema |

API 文档中心可以索引三类契约，但不得把 protobuf 或 CRD 伪装成 Console REST operation。

### 4.2 认证相关 HTTP 语义

本轮保持全部 operation 的现有匿名行为。未来启用认证时：

- 未提供或无效凭据返回 401；身份有效但权限不足返回 403。
- 认证错误继续使用 Console 既有 JSON error envelope。
- 是否保护 API docs 必须由部署策略明确决定，不能随业务路由隐式变化。
- 认证和授权变更属于独立功能计划，需要重新进行安全设计与统一确认。

## 5. 数据设计

本次没有数据库 schema、迁移或数据回填。目录迁移后数据库包的类型、表名、SQL、事务边界和 testcontainers 行为保持不变。

未来认证若需要持久化用户、角色、会话或审计信息，必须另立数据设计；本设计不预留空表或抽象 repository。

## 6. 安全性设计

- 公共 CRD 包只包含 API schema，不包含凭据、数据库或部署实现。
- Gateway 的组件认证与 Console 用户认证保持分离，防止错误复用信任模型。
- 认证未启用时文档不得声称 operation 已受保护。
- 未来 principal 只能由受信任中间件写入 context；handler 不接受客户端自报身份字段。
- 依赖边界检查防止 member plane 直接导入 gateway plane 实现。

## 7. 部署与运行设计

目录归组不改变二进制名、镜像、环境变量、端口、Helm values、探针或部署拓扑。最终验证需要确认 Dockerfile、Makefile 和 charts 中不存在旧 import/路径假设，但本次设计不要求改动部署行为。

## 8. 可测试性与验证策略

为减少重复执行和输出，每阶段只运行受影响包的编译或窄测试；完整回归集中在最后。

阶段内最低要求：

- 目录移动后受影响包必须可编译。
- 公共 CRD API 需运行其自身测试并编译直接调用方。
- Gateway 边界签名若变化，运行 Gateway/hublink 窄测试。
- Console 结构若变化，运行相关 handler/service 测试。
- 新增门禁脚本必须有一个覆盖失败和成功路径的最小测试。

最终统一执行 Swagger/API lint、契约兼容、Go lint/build/test、前端测试和构建、Helm lint/template。`make test-all`、Kind 和 E2E 只在具备部署授权与环境时执行。

## 9. 兼容、迁移与回滚

- 每个领域独立迁移；目录移动和行为调整分 Commit。
- 不保留旧 internal import 的兼容 wrapper，避免双事实来源。
- 公共 CRD API 迁移以结构化 schema 比较为门禁。
- database/admission/platform 迁移只允许 package/import 和必要生成引用变化。
- 每个阶段可单独 revert；没有数据库回滚要求。
- 任何契约差异、循环依赖或未解释行为变化都必须停止该阶段，不能推迟到最终回归解决。

## 10. 风险分析

| 风险 | 影响 | 控制措施 |
| --- | --- | --- |
| CRD 移动导致 schema 漂移 | 集群升级或控制器兼容失败 | 生成前后结构化比较；禁止同期改字段 |
| 大量 import 变更掩盖逻辑修改 | 审查困难 | 每领域单独 Commit；机械移动与逻辑改动分离 |
| operator/hublink 引用 gateway 实现 | 跨 plane 耦合继续扩大 | 增加禁止 import 门禁 |
| Console 路由分组改变行为 | 产生未授权访问或兼容问题 | 本轮保持匿名行为；路由覆盖检查 |
| 公开 Go API 后兼容成本增加 | 限制后续字段演进 | 明确版本策略；不公开数据库/实现类型 |
| 最终集中回归发现累积错误 | 修复成本上升 | 每阶段至少编译直接影响包；契约差异立即阻断 |

## 11. 已确认决策

1. 采用推荐的边界治理方案，不做完整平台化重构。
2. `api/kubesentry/v1alpha1` 是仓库外可导入的公共 Go API。
3. Gateway 继续只提供 gRPC。
4. 认证本轮只做到认证就绪，不实现具体协议。
5. 不拆五个 `cmd`，不抽取 `internal/testutil`。
6. 不建立 `pkg`；`internal/logging` 保持仓库私有。
7. 阶段内执行最小验证，最终再做一次完整回归。

## 12. 实施计划

详细任务、文件范围、验证命令、Commit 划分和回滚方式见：

- `docs/superpowers/plans/2026-09-23-project-boundary-and-auth-readiness-plan.md`
