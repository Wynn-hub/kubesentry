import { describe, expect, it } from 'vitest'
import userView from '../UserListView.vue?raw'
import activateView from '../ActivateUserView.vue?raw'
import loginView from '../LoginView.vue?raw'

describe('identity and tenant management surfaces', () => {
  it('uses explicit user lifecycle actions and one-time activation details', () => {
    expect(userView).toContain("command=\"activation\"")
    expect(userView).toContain("command=\"logout\"")
    expect(userView).toContain('activationOnceHint')
    expect(userView).toContain('activationUsername')
    expect(userView).toContain('username=${encodeURIComponent(activationUsername.value)}')
    expect(activateView).toContain("route.query.username")
    expect(loginView).toContain("route.query.username")
    expect(userView).not.toContain('<el-switch :model-value="row.status')
  })

})
