import { apiClient } from './generated/client'
import type { components } from './generated/console-api'
import { extract } from './http'

export type Tenant = components['schemas']['dto.TenantView']
export type User = components['schemas']['dto.UserView']
export type CreateUserRequest = components['schemas']['dto.CreateUserRequest']
export type CreateUserResult = components['schemas']['dto.CreateUserResult']
export type SecuritySettings = components['schemas']['dto.SecuritySettingsView']
export type SecuritySettingsInput = components['schemas']['dto.SecuritySettingsInput']

export const listTenants = () => extract<'listTenants'>(apiClient.GET('/tenants'))
export const createTenant = (name: string) => extract<'createTenant'>(apiClient.POST('/tenants', { body: { name } }))
export const setTenantStatus = (id: string, status: 'active' | 'disabled') => extract<'setTenantStatus'>(apiClient.PUT('/tenants/{id}/status', { params: { path: { id } }, body: { status } }))
export const syncTenantClusters = (id: string, clusterNames: string[], allowTransfer = false) =>
  extract<'syncTenantClusters'>(apiClient.PUT('/tenants/{id}/clusters', { params: { path: { id } }, body: { clusterNames, allowTransfer } }))
export const listUsers = () => extract<'listUsers'>(apiClient.GET('/users'))
export const createUser = (body: CreateUserRequest) => extract<'createUser'>(apiClient.POST('/users', { body }))
export const setUserStatus = (id: string, status: 'active' | 'disabled') => extract<'setUserStatus'>(apiClient.PUT('/users/{id}/status', { params: { path: { id } }, body: { status } }))
export const resetUserPassword = (id: string, password: string) => extract<'resetUserPassword'>(apiClient.POST('/users/{id}/reset-password', { params: { path: { id } }, body: { password } }))
export const regenerateUserActivation = (id: string) => extract<'regenerateUserActivation'>(apiClient.POST('/users/{id}/activation', { params: { path: { id } } }))
export const unlockUser = (id: string) => extract<'unlockUser'>(apiClient.POST('/users/{id}/unlock', { params: { path: { id } } }))
export const forceLogoutUser = (id: string) => extract<'forceLogoutUser'>(apiClient.POST('/users/{id}/logout', { params: { path: { id } } }))
export const forceUserPasswordChange = (id: string) => extract<'forceUserPasswordChange'>(apiClient.POST('/users/{id}/force-password-change', { params: { path: { id } } }))
export const deleteUser = (id: string) => extract<'deleteUser'>(apiClient.DELETE('/users/{id}', { params: { path: { id } } }))
export const getSecuritySettings = () => extract<'getSecuritySettings'>(apiClient.GET('/security-settings'))
export const updateSecuritySettings = (body: SecuritySettingsInput) => extract<'updateSecuritySettings'>(apiClient.PUT('/security-settings', { body }))
