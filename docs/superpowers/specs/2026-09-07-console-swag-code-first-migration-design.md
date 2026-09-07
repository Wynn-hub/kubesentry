# Console API Swag Code-First Migration — 概要设计

## 目的

将 KubeSentry Console HTTP API 从当前 Ogen/OpenAPI-first 生成链路迁移到
`github.com/swaggo/swag` code-first 链路。迁移保持现有 REST 路径、请求语义、响应 envelope、状态码和 Gateway protobuf/成员集群 CRD 契约不变，只替换 Console HTTP API 的描述与路由适配方式。

## 术语定义

| 简称/术语 | 说明 |
| --- | --- |
| Console API | `cmd/console` 提供的管理面 HTTP API |
| Swag | `github.com/swaggo/swag`，从 Go 注释生成 Swagger 2.0 文档 |
| Ogen | 当前从 OpenAPI 生成 Console DTO、路由和客户端代码的工具链 |
| Gateway API | `api/proto/gateway/v1/gateway.proto` 定义的 gRPC 契约 |
| CRD API | 成员集群侧 Kubernetes CustomResourceDefinition 契约 |

## 参考资料

| 序号 | 文档名称 | 来源 |
| --- | --- | --- |
| 1 | API 契约标准化设计 | `docs/superpowers/specs/2026-08-03-api-contract-standardization-design.md` |
| 2 | Go project layout | `golang-standards/project-layout` |
| 3 | Swag 官方文档 | `github.com/swaggo/swag` |

# 系统概述

## 系统目标

1. Console API 的接口描述以 Go handler 注释为唯一 HTTP 文档来源。
2. `make swagger` 可安装/调用固定版本的 swag，并生成 `api/swagger/swagger.yaml`，无需手工维护 YAML。
3. 现有 50 个 Console operation 的路径、方法、参数、响应 envelope、状态码和错误语义保持兼容。
4. API 文档中心继续展示 Console HTTP、Gateway protobuf、成员 CRD 三类契约，但不把后两者伪装成 REST。

## 设计约束

- 保留 `/cmd`、`/internal`、`/pkg`、`/api` 项目布局；本迁移不重构无关模块。
- HTTP API 使用 Swag 生成的 Swagger 2.0 文档；OpenAPI-first/Ogen 不再作为 Console HTTP API 的生产生成链路。
- Swag 的 Swagger 2.0 YAML 是源产物；因现有 Scalar 文档中心使用 OpenAPI 3，构建阶段必须将该 YAML 可重复转换为文档中心使用的 OpenAPI 3 JSON/YAML，转换工具与门禁在实施计划中固定，运行时不做源码扫描。
- Gateway gRPC 仍以 protobuf 为源；成员侧仍以 CRD 为源。
- 保留现有认证说明：Console 当前没有 HTTP auth middleware，`bearerAuth` 只能作为预留文档信息。
- 生成文件不得手工编辑；生成命令必须可重复且可在 CI/容器构建中执行。
- 数据库结构不变，不新增迁移版本。

## 设计思路

采用分阶段兼容迁移：先在现有 handler 上补齐 Swag 注释和稳定 DTO，再增加标准路由注册与文档生成；完成前保留 Ogen 适配层用于回归比对，确认行为一致后删除 Ogen Console 产物、生成客户端和相关门禁。这样可以把文档迁移与业务行为迁移分离，降低一次性替换风险。

备选方案：

- 双链路长期并存：兼容性高，但会形成两份接口定义，容易漂移，拒绝采用。
- 继续 Ogen、只额外导出 Swag：改动最小，但不满足 code-first 成为正式来源，拒绝采用。

## 系统总体架构

```text
Go handlers + Swag annotations
          │
          ├── swag init --outputTypes yaml
          ▼
api/swagger/swagger.yaml ──► build-time Swagger 2.0 → OpenAPI 3 ──► Console API docs / Scalar
          │
          ▼
net/http router ──► typed service ──► repository ──► PostgreSQL

Gateway gRPC ──► api/proto/gateway/v1/gateway.proto (unchanged)
Member cluster ──► Kubernetes CRDs (unchanged)
```

## 系统运行环境

- Go 1.26.x，`net/http` 与项目现有服务层。
- Node/npm 仅用于前端测试和文档页面，不作为 Swagger 生成器。
- `swag` 固定版本通过 Makefile 安装或调用；生成结果写入 `api/swagger/`。
- Console API 与 web 镜像均通过 Helm 部署。

# 功能设计

## 模块设计

### 1. Swagger 元数据与生成

- 在 Console 入口增加 `@title`、`@version`、`@description`、`@host`、`@BasePath`。
- 为 50 个 handler 方法补充 `@Summary`、`@Description`、`@Tags`、`@Accept`、`@Produce`、`@Param`、`@Success`、`@Failure`、`@Router`。
- 建立 `make swagger`、`make swagger-check`，检查生成结果与提交文件一致。

### 2. HTTP 路由与 DTO

- 将 Ogen 路由适配替换为标准路由注册；业务调用继续进入现有 service。
- 请求和响应 DTO 放在 `internal/console/model` 或 handler 专用类型中，使用稳定 JSON tags。
- 统一错误 envelope，内部错误对客户端返回稳定消息，原始错误仅进入服务端日志。

### 3. 前端 API 客户端

- 删除对 Ogen/openapi-typescript 生成客户端的运行时依赖。
- 以 Swagger 文档对应的 TypeScript DTO 作为过渡类型来源；保留现有模块函数和页面调用接口。
- 文档中心读取生成的 `api/swagger/swagger.yaml` 的可访问 JSON/YAML 版本，或由后端转换为 Scalar 可消费格式。

### 4. 其他契约

- `api/proto/gateway/v1/gateway.proto`、protobuf 生成代码和 Gateway 文档不变。
- `charts/kubesentry` CRD manifests、operator/webhook 客户端不变。
- `api/openapi.yaml` 改为明确标记 Console Swagger、Gateway protobuf、CRD 三类来源，不再作为 Console 生成源。

## 接口设计

迁移必须保留以下兼容性：

- 路径和 HTTP 方法不变。
- 分页参数、排序参数、筛选参数和 CSV 数组序列化不变。
- 成功响应仍使用 `code: 0` 和 `data` envelope。
- 4xx/5xx 状态码和错误响应字段不变；未知内部错误返回稳定 `request failed`。
- 集群安装命令、token 重签、卸载任务及相关事务语义不变。

## 界面设计

API 文档中心保留现有 `/system/api-docs` 路由和 Scalar 展示；页面分为 Console Swagger、Gateway protobuf、Kubernetes CRD 三个区域，并明确认证现状和契约文件位置。

# 数据库设计

不涉及数据库表、索引或迁移版本变更。

# 非功能设计

## 性能

Swagger 文档在构建时生成并作为静态资源/嵌入文件提供，运行时不扫描源码、不访问数据库。

## 可靠性

生成漂移检查、50 路由覆盖检查、Swagger 结构校验和 API smoke test 必须纳入 `make api-lint`；`test-all` 必须依赖该门禁。

## 安全性

禁止把 token、密码、私钥写入 Swagger example。Console 当前无 HTTP 认证 middleware，文档不得宣称已启用认证。

## 升级/补丁

Swag 版本固定；升级时先运行生成、契约检查、Go/前端测试，再更新生成文件。旧 Ogen 产物删除前必须完成兼容性比对。

## 安装部署

Dockerfile 和 Helm 只复制生成后的 Swagger 产物，不在运行容器中安装 swag 或扫描源码。

## 运维设计

启动日志记录 Swagger 文档版本和生成时间；文档 endpoint 提供健康的静态响应，不依赖数据库可用性。

## 兼容性

迁移期间保持现有 API 行为和前端页面；Gateway/CRD 兼容边界不变。若发现无法保持的 operation，必须在迁移计划中单独列出并暂停删除 Ogen。

## 其他

本设计不包含用户认证、多租户、Gateway protobuf 改造或成员侧 CRD 改造。
