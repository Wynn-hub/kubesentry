import { apiClient } from './generated/client'
import type { components } from './generated/console-api'
import { extract } from './http'

export type Me = components['schemas']['dto.MeView']

export const getSetupStatus = () => extract<'getSetupStatus'>(apiClient.GET('/setup/status'))
export const setupAdmin = (password: string) => extract<'setupAdmin'>(apiClient.POST('/setup/admin', { body: { password } }))
export const login = (username: string, password: string) => extract<'login'>(apiClient.POST('/auth/login', { body: { username, password } }))
export const activateUser = (code: string, password: string) => extract<'activateUser'>(apiClient.POST('/auth/activate', { body: { code, password } }))
export const changePassword = (currentPassword: string, newPassword: string) => extract<'changePassword'>(apiClient.POST('/auth/change-password', { body: { currentPassword, newPassword } }))
export const logout = () => extract<'logout'>(apiClient.POST('/auth/logout'))
export const getMe = () => extract<'getCurrentUser'>(apiClient.GET('/me'))
