package handler

import "github.com/Wynn-hub/kubesentry/internal/console/dto"

var _ = dto.LoginInput{}

// setupStatus documents GET /setup/status.
// @ID getSetupStatus
// @Summary 查询首次部署初始化状态
// @Description 返回是否仍需为默认 admin 用户设置初始密码。
// @Tags Auth
// @Produce json
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.SetupStatusView}
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /setup/status [get]
func setupStatus() {}

// setupAdmin documents POST /setup/admin.
// @ID setupAdmin
// @Summary 首次设置 admin 密码
// @Description 原子设置默认 admin 用户密码；完成后再次调用返回冲突。
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body dto.PasswordInput true "管理员密码"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Failure 500 {object} dto.SwagErrorResponse
// @Router /setup/admin [post]
func setupAdmin() {}

// login documents POST /auth/login.
// @ID login
// @Summary 登录并建立会话
// @Description 校验本地用户凭据并通过 HttpOnly Cookie 建立会话。
// @Tags Auth
// @Accept json
// @Produce json
// @Param body body dto.LoginInput true "登录凭据"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 429 {object} dto.SwagErrorResponse
// @Router /auth/login [post]
func login() {}

// logout documents POST /auth/logout.
// @ID logout
// @Summary 注销当前会话
// @Description 撤销当前数据库会话并清除浏览器 Cookie。
// @Tags Auth
// @Produce json
// @Security SessionCookie
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Router /auth/logout [post]
func logout() {}

// me documents GET /me.
// @ID getCurrentUser
// @Summary 查询当前登录用户
// @Description 返回当前用户、角色和可选租户归属。
// @Tags Auth
// @Produce json
// @Security SessionCookie
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.MeView}
// @Failure 401 {object} dto.SwagErrorResponse
// @Router /me [get]
func me() {}

// listTenants documents GET /tenants.
// @ID listTenants
// @Summary 查询租户列表
// @Description 系统管理员查询全部租户。
// @Tags Tenant
// @Produce json
// @Security SessionCookie
// @Success 200 {object} dto.SwagOptionalResponse{data=[]dto.TenantView}
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Router /tenants [get]
func listTenants() {}

// createTenant documents POST /tenants.
// @ID createTenant
// @Summary 创建租户
// @Description 系统管理员创建单租户边界。
// @Tags Tenant
// @Accept json
// @Produce json
// @Security SessionCookie
// @Param body body dto.CreateTenantRequest true "租户"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.TenantView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Router /tenants [post]
func createTenant() {}

// setTenantStatus documents PUT /tenants/{id}/status.
// @ID setTenantStatus
// @Summary 设置租户状态
// @Description 系统管理员启用或禁用租户。
// @Tags Tenant
// @Accept json
// @Produce json
// @Security SessionCookie
// @Param id path int true "租户 ID"
// @Param body body dto.TenantStatusRequest true "状态"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Router /tenants/{id}/status [put]
func setTenantStatus() {}

// listUsers documents GET /users.
// @ID listUsers
// @Summary 查询可管理用户
// @Description 系统管理员查询全部用户，租户管理员仅查询本租户用户。
// @Tags User
// @Produce json
// @Security SessionCookie
// @Success 200 {object} dto.SwagOptionalResponse{data=[]dto.UserView}
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Router /users [get]
func listUsers() {}

// createUser documents POST /users.
// @ID createUser
// @Summary 创建用户
// @Description 系统管理员创建任意受支持角色，租户管理员仅创建本租户普通用户。
// @Tags User
// @Accept json
// @Produce json
// @Security SessionCookie
// @Param body body dto.CreateUserRequest true "用户"
// @Success 200 {object} dto.SwagOptionalResponse{data=dto.UserView}
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Router /users [post]
func createUser() {}

// setUserStatus documents PUT /users/{id}/status.
// @ID setUserStatus
// @Summary 设置用户状态
// @Description 禁用用户时同步撤销其全部会话。
// @Tags User
// @Accept json
// @Produce json
// @Security SessionCookie
// @Param id path int true "用户 ID"
// @Param body body dto.UserStatusRequest true "状态"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Router /users/{id}/status [put]
func setUserStatus() {}

// resetUserPassword documents POST /users/{id}/reset-password.
// @ID resetUserPassword
// @Summary 重置用户密码并撤销会话
// @Description 设置新密码并撤销目标用户的全部现有会话。
// @Tags User
// @Accept json
// @Produce json
// @Security SessionCookie
// @Param id path int true "用户 ID"
// @Param body body dto.ResetPasswordRequest true "新密码"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Router /users/{id}/reset-password [post]
func resetUserPassword() {}

// deleteUser documents DELETE /users/{id}.
// @ID deleteUser
// @Summary 删除用户并撤销会话
// @Description 删除可管理用户并撤销其全部现有会话。
// @Tags User
// @Produce json
// @Security SessionCookie
// @Param id path int true "用户 ID"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Router /users/{id} [delete]
func deleteUser() {}

// assignClusterTenant documents PUT /clusters/{name}/tenant.
// @ID assignClusterTenant
// @Summary 分配或解除集群租户
// @Description 系统管理员设置集群唯一租户归属；仍被租户分发引用时拒绝转移。
// @Tags Cluster
// @Accept json
// @Produce json
// @Security SessionCookie
// @Param name path string true "集群名称"
// @Param body body dto.ClusterTenantRequest true "租户归属"
// @Success 200 {object} dto.SwagEmptyResponse
// @Failure 400 {object} dto.SwagErrorResponse
// @Failure 401 {object} dto.SwagErrorResponse
// @Failure 403 {object} dto.SwagErrorResponse
// @Failure 404 {object} dto.SwagErrorResponse
// @Failure 409 {object} dto.SwagErrorResponse
// @Router /clusters/{name}/tenant [put]
func assignClusterTenant() {}
