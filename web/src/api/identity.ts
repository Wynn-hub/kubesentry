import { apiClient } from './generated/client'
import type { components } from './generated/console-api'
import { extract } from './http'

export type Tenant = components['schemas']['dto.TenantView']
export type User = components['schemas']['dto.UserView']
export type CreateUserRequest = components['schemas']['dto.CreateUserRequest']

export const listTenants = () => extract<'listTenants'>(apiClient.GET('/tenants'))
export const createTenant = (name: string) => extract<'createTenant'>(apiClient.POST('/tenants', { body: { name } }))
export const setTenantStatus = (id: number, status: 'active' | 'disabled') => extract<'setTenantStatus'>(apiClient.PUT('/tenants/{id}/status', { params: { path: { id } }, body: { status } }))
export const listUsers = () => extract<'listUsers'>(apiClient.GET('/users'))
export const createUser = (body: CreateUserRequest) => extract<'createUser'>(apiClient.POST('/users', { body }))
export const setUserStatus = (id: number, status: 'active' | 'disabled') => extract<'setUserStatus'>(apiClient.PUT('/users/{id}/status', { params: { path: { id } }, body: { status } }))
export const resetUserPassword = (id: number, password: string) => extract<'resetUserPassword'>(apiClient.POST('/users/{id}/reset-password', { params: { path: { id } }, body: { password } }))
export const deleteUser = (id: number) => extract<'deleteUser'>(apiClient.DELETE('/users/{id}', { params: { path: { id } } }))
