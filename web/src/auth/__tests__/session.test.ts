import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../api/auth', () => ({
  getSetupStatus: vi.fn(), getMe: vi.fn(), logout: vi.fn(),
}))

import { getMe, getSetupStatus } from '../../api/auth'
import { authState, can, refreshAuth } from '../session'

describe('session state', () => {
  beforeEach(() => { authState.status = 'unknown'; authState.me = null; vi.resetAllMocks() })

  it('enters setup before requesting the current user', async () => {
    vi.mocked(getSetupStatus).mockResolvedValue({ required: true })
    await refreshAuth()
    expect(authState.status).toBe('setup')
    expect(getMe).not.toHaveBeenCalled()
  })

  it('loads the authenticated principal and capabilities', async () => {
    vi.mocked(getSetupStatus).mockResolvedValue({ required: false })
    vi.mocked(getMe).mockResolvedValue({ id: 1, username: 'admin', role: 'system_admin' })
    await refreshAuth()
    expect(authState.status).toBe('authenticated')
    expect(can('platform.manage')).toBe(true)
  })
})
