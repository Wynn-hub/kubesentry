# KubeSentry 项目边界治理与认证就绪实施计划

> 实施时按阶段执行。每阶段只做最小阻断验证；全部阶段完成后再运行一次完整回归。

**目标：** 在保持 Console REST、Gateway gRPC、成员 CRD、数据库及部署行为兼容的前提下，公开成员 CRD Go API，按领域整理内部包，收紧跨 plane 依赖，并使 Console 具备未来认证的清晰接入位置。

**设计依据：** `docs/superpowers/specs/2026-09-23-project-boundary-and-auth-readiness-design.md`

**明确非目标：** 不实现认证；不增加 Gateway REST；不拆五个 `cmd`；不建立 `pkg/common/utils`；不抽取 `internal/testutil`；不修改数据库 schema、protobuf RPC/field、CRD group/version 或现有 HTTP 行为。

## 实施授权前提

本计划目前仅供审查。开始修改生产代码前，必须按仓库统一确认流程重新确认：

1. 开发计划是否批准；
2. 是否授权计划内修改、格式化、生成和验证；
3. 本地 Commit 模式；
4. 允许提交时的 Commit Message 语言与格式。

现有 `docs/api-prompt.txt` 及其他未提交内容均视为用户工作，实施期间不得覆盖、删除或混入计划 Commit。

## 全局实施规则

- 一个阶段只解决一种边界问题，机械移动与行为修改不得混入同一 Commit。
- 移动前搜索所有生产/测试调用方、生成脚本、Makefile、Dockerfile、charts 和文档引用。
- 首选 `git mv` 保留历史；只暂存本阶段文件，不使用 `git add .` 或 `git add -A`。
- 不保留旧 import wrapper；迁移阶段内一次更新全部仓库内调用方。
- 阶段内不运行全量 `make test`、`make lint`、前端全测或 Helm 全验，只运行下述最小门禁。
- 最小门禁失败必须在当前阶段修复；不能把编译错误、契约漂移或循环依赖累积到最终回归。
- 直接执行 Go 命令时遵循 `CLAUDE.md` 指定的 Go 工具链环境。

## Task 1：阶段 0 — 重新冻结当前基线

### 目标

在目录迁移前记录当前契约、依赖和工作区状态，确保后续比较基于已完成的 Swag 架构，而非旧 Ogen 计划。

### 前置条件

- 已取得实施授权。
- 工作区已有修改已识别并排除。

### 涉及文件

- Read: `api/swagger/swagger.yaml`
- Read: `internal/console/openapi_bundle.json`
- Read: `api/proto/gateway/v1/gateway.proto`
- Read: `charts/kubesentry/crds/*.yaml`
- Read: 五个 `cmd/*/main.go`
- Optional create/update: 现有依赖/契约检查 fixture；只有当前脚本没有可重复基线时才增加

### 具体工作

- [ ] 记录当前分支、`git status --short` 和用户已有修改。
- [ ] 记录 Console path/method/operationId 数量及 OAS version。
- [ ] 记录 Gateway service/RPC、protobuf package、`go_package` 和 descriptor 摘要。
- [ ] 记录成员 CRD group/version/kind 与结构化 schema 摘要。
- [ ] 搜索旧目录的全部生产、测试、生成和部署引用，形成阶段清单。
- [ ] 确认当前不存在 `internal/api/v1alpha1` 之外的 CRD Go 类型事实来源。

### 保持不变

所有代码、契约、生成物和运行行为。

### 最小验证

~~~bash
git status --short
python3 scripts/check_openapi_contract.py
git diff --check
~~~

只读取/运行现有快速契约检查，不执行生成和全量测试。

### 完成标准

- 基线可机器比较；调用方清单覆盖生产、测试和生成入口。
- 用户已有修改已明确排除。

### 回滚

若未创建 fixture，无需回滚；若补充 fixture，仅 revert 该独立 Commit。

### 建议 Commit

仅在确需更新基线 fixture 时提交：`test(architecture): refresh package and contract baseline`

## Task 2：阶段 1 — 公开成员 CRD Go API

### 目标

将成员 CRD 类型从仓库私有包迁移为正式公共 Go API，不改变 Kubernetes 契约。

### 前置条件

阶段 0 完成。

### 涉及目录和文件

- Move: `internal/api/v1alpha1/*` -> `api/kubesentry/v1alpha1/*`
- Modify: operator、webhook、hublink、console、gateway、database、groupresolve 及测试中的 import
- Modify if referenced: code generation markers/scripts、Makefile target、CRD generation configuration
- Compare only: `charts/kubesentry/crds/*.yaml`

### 具体工作

- [ ] 使用 `git mv` 迁移现有类型、Scheme 注册、managed metadata、DeepCopy 和测试。
- [ ] 保持 package 名 `v1alpha1`、API group/version、Kind、JSON tag 和 Kubebuilder marker。
- [ ] 将所有仓库内 import 更新为 `github.com/Wynn-hub/kubesentry/api/kubesentry/v1alpha1`。
- [ ] 检查 Gateway/Console 仅使用 schema/spec；记录并移除任何对 Kubernetes client、Scheme 或 reconciler 的不必要依赖。
- [ ] 校验公共包没有导入 `internal/*`。
- [ ] 更新面向外部开发者的 Go import 示例和 API 版本承诺。
- [ ] 仅在仓库现有流程要求时重新生成 DeepCopy/CRD；生成结果必须与基线结构化等价。
- [ ] 删除空的 `internal/api/v1alpha1` 目录，不添加兼容 wrapper。

### 保持不变

- CRD group/version/kind、字段、required、default、validation 和 printer columns。
- operator/webhook 控制逻辑。
- Gateway protobuf、Console REST、数据库模型和部署资源。

### 最小验证

~~~bash
go test ./api/kubesentry/v1alpha1 -count=1
go test ./internal/operator/... ./internal/webhook/... ./internal/operator/hublink/... -run '^$'
go test ./internal/console/... ./internal/gateway/... ./internal/groupresolve/... -run '^$'
git diff --check
~~~

如生成了 CRD，再运行现有 CRD 生成漂移比较；不运行全量测试。

### 完成标准

- 新公共 import 可被独立 Go 包使用。
- 旧 import 在生产代码、测试和生成配置中为零。
- 公共包不依赖 `internal`。
- CRD schema 与阶段 0 基线无语义差异。

### 回滚

单独 revert 本阶段 Commit；不涉及数据库或运行数据。

### 建议 Commit

`refactor(api): publish member CRD Go types`

## Task 3：阶段 2 — 数据库领域归组

### 目标

将散落的数据库包归入稳定的仓库内领域目录，不改变数据行为。

### 前置条件

阶段 1 已完成并通过最小验证。

### 涉及目录和文件

- Move: `internal/dbmodel` -> `internal/database/model`
- Move: `internal/dbrepo` -> `internal/database/repository`
- Move: `internal/dbmigrate` -> `internal/database/migrate`
- Modify: `cmd/console`、`cmd/gateway`、`cmd/db` 及所有生产/测试 import
- Modify if referenced: scripts、Makefile、Dockerfile、charts、文档中的 Go package 路径

### 具体工作

- [ ] 先迁移 model 并更新调用方，保持包名简洁为 `model`。
- [ ] 再迁移 repository，保持所有 repository API、GORM model、事务和错误语义。
- [ ] 最后迁移 migrate，原样保留 embedded SQL 路径和 migration 版本。
- [ ] 更新 testcontainers 测试引用，但不抽取公共 test helper。
- [ ] 搜索并清除三个旧 import path。
- [ ] 检查 console、gateway、db 均直接依赖所需数据库子包，不经其他组件转发。

### 保持不变

数据库 schema、migration SQL、表名、GORM tag、事务、查询、分页和 seed 行为。

### 最小验证

~~~bash
go test ./internal/database/model ./internal/database/repository ./internal/database/migrate -run '^$'
go test ./cmd/console ./cmd/gateway ./cmd/db -run '^$'
git diff --check
~~~

本阶段不启动 PostgreSQL testcontainers；其完整验证放在最终回归。

### 完成标准

- 三个旧目录和 import path 为零。
- console、gateway、db 及新数据库包可编译。
- SQL 和 migration 文件内容无非路径性变化。

### 回滚

按 model、repository、migrate 的独立 Commit 逆序 revert。

### 建议 Commit

1. `refactor(database): group shared models`
2. `refactor(database): group repositories`
3. `refactor(database): group migrations`

## Task 4：阶段 3 — Admission 与 Platform 领域归组

### 目标

改善顶层 `internal` 导航，不改变领域职责或 API。

### 前置条件

阶段 2 完成。

### 涉及目录

- Move: `internal/admissionobs` -> `internal/admission/observability`
- Move: `internal/admissionpolicy` -> `internal/admission/policy`
- Move: `internal/platformobs` -> `internal/platform/observability`
- Modify: webhook、operator/hublink、gateway、console、database repository 及测试 import

### 具体工作

- [ ] 每个包独立移动并更新其全部调用方。
- [ ] 将 package 名分别改为 `observability`、`policy`、`observability`；调用处通过 import path 上下文或必要 alias 消除歧义。
- [ ] 不创建聚合 package，不增加 facade 或转发 wrapper。
- [ ] 搜索旧路径并检查没有生产、测试、文档或生成引用遗漏。

### 保持不变

事件结构、匹配规则、采样、wire 转换、持久化和上报行为。

### 最小验证

~~~bash
go test ./internal/admission/... ./internal/platform/... -run '^$'
go test ./internal/webhook/... ./internal/operator/... ./internal/gateway/... ./internal/console/... -run '^$'
git diff --check
~~~

### 完成标准

- 三个旧路径为零。
- 直接消费者全部可编译。
- 没有新增聚合层或循环依赖。

### 回滚

三个领域 Commit 可分别 revert。

### 建议 Commit

1. `refactor(admission): group observability package`
2. `refactor(admission): group policy package`
3. `refactor(platform): group observability package`

## Task 5：阶段 4 — 收紧 Gateway 与 member plane 边界

### 目标

确保 operator/hublink 只通过 protobuf/认证附属协议桥接 Gateway，不依赖 Gateway 服务或数据库实现。

### 前置条件

前三个目录迁移阶段完成，import 已稳定。

### 涉及目录和文件

- Audit/Modify: `internal/operator/hublink/*`
- Audit/Modify: `internal/gateway/*`
- Audit/Modify: `internal/gatewaypb/metadata.go`、`topics.go`
- Audit/Modify: `internal/gatewayauth/*`
- Audit/Modify: `internal/gatewaymodel/*`
- Audit/Modify: CRD render 的实际调用方和测试

### 具体工作

- [ ] 用 import/构造调用证据确认 hublink 不依赖 `internal/gateway` 和 `internal/database/repository`。
- [ ] wire metadata/topic 常量继续紧邻 `gatewaypb`；认证 token 规则继续位于 `gatewayauth`。
- [ ] Gateway plane 的 Cluster/PolicyDistribution 继续位于 `gatewaymodel`，不迁入公共 CRD API。
- [ ] 检查 render 函数是否接收完整 CRD 但只使用 Spec；只有证据明确时缩窄参数。
- [ ] 若需要调整 render 签名，先补/调整窄测试，再修改全部调用方；不要顺带改渲染行为。
- [ ] 增加或更新最小依赖门禁，禁止 hublink import Gateway/database 实现。

### 保持不变

Gateway 只提供 gRPC；RPC、protobuf field、metadata key、token 语义、render 结果和数据库行为不变。

### 最小验证

~~~bash
go test ./internal/operator/hublink/... ./internal/gateway/... ./internal/gatewayauth ./internal/gatewaymodel ./internal/gatewaypb -count=1
git diff --check
~~~

若最终没有真实反向依赖或签名收益，本阶段只提交依赖门禁，不做纯审美重构。

### 完成标准

- operator/hublink 不依赖 Gateway/database 实现。
- 合法跨 plane 依赖只剩公共 CRD API、`gatewaypb`、`gatewayauth` 及必要领域类型。
- Gateway gRPC 契约无变化。

### 回滚

门禁与签名调整分别提交，可独立 revert。

### 建议 Commit

1. `test(architecture): enforce gateway plane boundaries`
2. 仅在确需缩窄签名时：`refactor(gateway): narrow CRD render inputs`

## Task 6：阶段 5 — 稳定 Console 内部边界

### 目标

在不改变 50 个 REST operation 和 Swag 契约的前提下，消除剩余的反向依赖、根目录业务堆积和无价值薄层。

### 前置条件

数据库新路径稳定；Gateway 边界审计完成。

### 涉及目录和文件

- Audit/Modify: `internal/console/*.go`
- Audit/Modify: `internal/console/dto/*`
- Audit/Modify: `internal/console/handler/*`
- Audit/Modify: `internal/console/service/*`
- Audit/Modify: `internal/console/repository/*`
- Modify as needed: `cmd/console/main.go`
- Compare only: `api/swagger/swagger.yaml`、`internal/console/openapi_bundle.json`

### 具体工作

- [ ] 建立根目录文件职责清单：server/router/docs/startup/transport glue 保留，业务用例下沉到既有 service。
- [ ] 检查 handler 只依赖 service/DTO，不直接依赖 database repository。
- [ ] 检查 service 不依赖 HTTP request/response 或 Gateway server 实现。
- [ ] 检查 repository 从新的 database repository 构造，不接收其他组件的装配 bundle。
- [ ] 保持全部 REST DTO 和 Swag schema 在 `internal/console/dto`。
- [ ] 仅删除无行为、无隔离、无测试价值的转发层；每项删除必须列出调用证据。
- [ ] 不为目录整齐移动已经合理的 transport glue，不创建新的 interface/factory。

### 保持不变

path、method、operationId、参数、JSON 字段、status、error envelope、分页和匿名访问行为。

### 最小验证

~~~bash
go test ./internal/console/handler/... ./internal/console/service/... ./internal/console/repository/... -count=1
go test ./internal/console -run 'Test.*(Route|Server|Response)' -count=1
python3 scripts/check_openapi_contract.py
git diff --check
~~~

本阶段不运行完整 Swagger 重生成；只要修改 Swag 注释或 DTO schema，就必须立即运行 `make swagger-check`，不能等到最终阶段。

### 完成标准

- Console 依赖方向可由 import 和构造流程验证。
- Swag 注释仍是唯一事实来源，无第二套 DTO/spec。
- 现有路由和 JSON 行为无变化。

### 回滚

根目录整理、依赖修复和薄层删除分别提交并可独立 revert。

### 建议 Commit

- `refactor(console): enforce internal dependency direction`
- 仅在确有薄层可删时：`refactor(console): remove redundant forwarding layer`

## Task 7：阶段 6 — 建立 Console 认证接入边界

### 目标

使未来认证可以在路由层接入，同时保证本轮没有认证行为变化。

### 前置条件

Console 内部分层已稳定。

### 涉及目录和文件

- Modify: `internal/console/server.go` 或当前实际 router 装配文件
- Modify/Test: `internal/console/server_test.go`、`route_assert_test.go`、相关 middleware 测试
- Review: `internal/console/dto/swagger.go`、handler Swag security 注释
- Document: Console 认证扩展约束

### 具体工作

- [ ] 在现有 router 装配中明确列出 public、protected、operational 路由组。
- [ ] 当前 protected 组继续使用匿名透传路径；不创建假认证中间件。
- [ ] 保持健康/就绪探针在 operational/public 范围。
- [ ] 增加最小路由分类测试，证明每个 operation 只注册一次且访问结果不变。
- [ ] 检查 `BearerAuth` 仅作为未来 security scheme；未启用认证的 operation 不标强制 security。
- [ ] 在架构文档记录未来 principal 通过 context 传播，以及 401/403/error envelope 约束。
- [ ] 不增加身份数据库表、Authenticator interface、provider factory、RBAC engine 或配置项。

### 保持不变

全部 endpoint 当前可访问性、HTTP response、部署配置和前端行为。

### 最小验证

~~~bash
go test ./internal/console -run 'Test.*(Route|Middleware|Server)' -count=1
python3 scripts/check_openapi_contract.py
git diff --check
~~~

### 完成标准

- 路由分类明确且有测试。
- 当前访问行为无变化。
- 未引入认证依赖或虚假 OpenAPI security 声明。

### 回滚

单独 revert 路由分类 Commit。

### 建议 Commit

`refactor(console): define authentication-ready route groups`

## Task 8：阶段 7 — 固化架构门禁与项目文档

### 目标

阻止旧路径和跨 plane 实现依赖重新进入仓库，并使文档反映当前 Swag 架构。

### 前置条件

所有目录和依赖调整完成。

### 涉及文件

- Create: `scripts/check_architecture_boundaries.py`
- Create: `scripts/check_architecture_boundaries_test.py`
- Modify: Makefile/CI 仅在现有检查入口无法复用时
- Modify: `docs/api-migration-report.md`
- Modify: 架构/开发者目录说明及公共 CRD import 示例
- Review: `api/openapi.yaml`

### 具体工作

- [ ] 新增一个只负责 Go import/路径边界的 Python stdlib 脚本；不把目录规则混入 OpenAPI 契约检查器。
- [ ] 门禁禁止旧 import：`internal/api/v1alpha1`、`internal/dbrepo`、`internal/dbmodel`、`internal/dbmigrate`、`internal/admissionobs`、`internal/admissionpolicy`、`internal/platformobs`。
- [ ] 门禁禁止 hublink 导入 `internal/gateway` 和 `internal/database/repository`。
- [ ] 门禁检查 `api/kubesentry/v1alpha1` 不导入 `internal/*`。
- [ ] 为门禁脚本增加最小测试：一个允许 fixture、一个拒绝 fixture。
- [ ] 修正 API 迁移报告中“前端仍消费旧 OAS3”等已过时描述。
- [ ] 明确 Console Swag、Gateway protobuf、Member CRD 三类唯一事实来源。
- [ ] 不删除 `api/schemas` 中 Gateway protobuf 或成员 CRD 的文档资源；删除任何文件前逐文件确认消费者为零。

### 保持不变

生成工具版本、运行契约、前端生成入口和部署行为。

### 最小验证

~~~bash
python3 scripts/check_architecture_boundaries_test.py
python3 scripts/check_architecture_boundaries.py
python3 scripts/check_openapi_contract.py
git diff --check
~~~

### 完成标准

- 旧路径和禁止依赖可被 CI 自动拒绝。
- API 迁移报告与当前代码一致。
- 三类契约文档资源均保留且可被文档中心消费。

### 回滚

文档和门禁分 Commit；门禁可独立 revert，不影响运行代码。

### 建议 Commit

1. `test(architecture): enforce package boundaries`
2. `docs(architecture): document public API and package layout`

## Task 9：阶段 8 — 最终集中回归

### 目标

一次性验证完整改造对 Go、契约、前端、Helm 和构建的影响。

### 前置条件

阶段 1–7 均完成最小验证；工作区只包含本计划变更和已识别的用户文件。

### 8.1 契约与生成验证

~~~bash
make swagger-check
make api-lint
python3 scripts/check_openapi_contract.py
~~~

另外按仓库现有命令执行 CRD 和 protobuf 生成漂移检查。比较要求：

- Console operation、schema、status 和派生 OAS3 无未解释漂移；
- Gateway RPC、protobuf field number 和 descriptor 无变化；
- Member CRD group/version/kind/schema 无语义变化。

### 8.2 Go 验证

~~~bash
make lint
make build
make test
~~~

`make test` 必须覆盖 Console handler/service、数据库 testcontainers、Gateway 和 operator/hublink。

### 8.3 前端验证

从 `web` 目录执行仓库锁定命令：

~~~bash
npm run test
npm run build
~~~

确认 openapi-typescript 消费派生 OAS3，Scalar 文档页可以加载三类契约索引。

### 8.4 Helm 与部署静态验证

按仓库现有 chart 名执行：

~~~bash
helm lint charts/kubesentry
helm lint charts/kubesentry-gateway
helm template kubesentry charts/kubesentry >/dev/null
helm template kubesentry-gateway charts/kubesentry-gateway >/dev/null
~~~

### 8.5 E2E 执行条件

`make test-all`、Kind、镜像构建和部署测试只在用户明确授权部署类操作且环境满足时执行。未获授权时必须记录未执行原因和剩余风险，不能声称通过。

### 8.6 最终审查

- [ ] 搜索旧目录/import 为零。
- [ ] 检查公共 API 不依赖 `internal`。
- [ ] 检查没有新增 `pkg/common/utils/shared/factory`。
- [ ] 检查没有认证实现、配置或虚假 security 标记。
- [ ] 检查 staged diff 不含 `docs/api-prompt.txt` 或其他用户修改。
- [ ] 检查不存在未完成占位标记、调试输出、生成垃圾和意外依赖。

### 完成标准

- 所有已授权完整验证成功，或未执行项有明确授权/环境原因。
- 三类契约保持兼容。
- 五个二进制构建成功。
- 最终目录和依赖符合设计文档。
- 用户已有修改完整保留。

### 回滚

若最终回归暴露单阶段问题，优先修复对应阶段；若无法安全修复，按阶段 Commit 逆序 revert。不得使用 `git reset --hard` 或覆盖用户工作。

## 建议 Commit 总览

1. 可选：`test(architecture): refresh package and contract baseline`
2. `refactor(api): publish member CRD Go types`
3. `refactor(database): group shared models`
4. `refactor(database): group repositories`
5. `refactor(database): group migrations`
6. `refactor(admission): group observability package`
7. `refactor(admission): group policy package`
8. `refactor(platform): group observability package`
9. `test(architecture): enforce gateway plane boundaries`
10. 可选：`refactor(gateway): narrow CRD render inputs`
11. `refactor(console): enforce internal dependency direction`
12. 可选：`refactor(console): remove redundant forwarding layer`
13. `refactor(console): define authentication-ready route groups`
14. `test(architecture): enforce package boundaries`
15. `docs(architecture): document public API and package layout`

实际实施时可以合并相邻的纯机械小 Commit，但不得把目录移动、行为调整、门禁和文档全部压成一个不可回滚 Commit。

## 计划完成后的后续独立事项

以下事项必须另行调查、设计和确认，不能以“扩展性”为由提前实施：

- Console OIDC/JWT/反向代理认证和授权模型；
- 用户、角色、会话或审计数据库；
- CRD `v1beta1`/`v1` 版本升级与 conversion webhook；
- 仓库外 Gateway Go SDK；
- 公共 logging 库或统一组件运行框架；
- `make test-all`、Kind 部署、发布、push 或 PR。
