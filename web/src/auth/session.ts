import { reactive } from 'vue'
import { ApiError, setUnauthorizedHandler } from '../api/http'
import { getMe, getSetupStatus, logout as requestLogout, type Me } from '../api/auth'

export type AuthStatus = 'unknown' | 'setup' | 'anonymous' | 'authenticated'
export type Capability = 'platform.manage' | 'tenant.manage' | 'resource.write' | 'resource.read'

export const authState = reactive<{ status: AuthStatus; me: Me | null }>({ status: 'unknown', me: null })

export function clearAuth() {
  authState.status = 'anonymous'
  authState.me = null
}

setUnauthorizedHandler(clearAuth)

export async function refreshAuth() {
  const setup = await getSetupStatus()
  if (setup.required) {
    authState.status = 'setup'
    authState.me = null
    return
  }
  try {
    authState.me = await getMe()
    authState.status = 'authenticated'
  } catch (error) {
    if (!(error instanceof ApiError) || error.status !== 401) throw error
    clearAuth()
  }
}

export async function signOut() {
  try {
    await requestLogout()
  } finally {
    clearAuth()
  }
}

export function can(capability: Capability) {
  const role = authState.me?.role
  if (!role) return false
  if (capability === 'resource.read') return true
  if (capability === 'platform.manage') return role === 'system_admin'
  if (capability === 'tenant.manage') return role === 'system_admin' || role === 'tenant_admin'
  return role !== 'viewer'
}

export function avatarInitials(username?: string) {
  const value = username?.trim()
  if (!value) return '?'
  return value.slice(0, 2).toUpperCase()
}
