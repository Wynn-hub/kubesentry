import { describe, expect, it } from 'vitest'
import userView from '../UserListView.vue?raw'

describe('identity and tenant management surfaces', () => {
  it('uses explicit user lifecycle actions and one-time activation details', () => {
    expect(userView).toContain("command=\"activation\"")
    expect(userView).toContain("command=\"logout\"")
    expect(userView).toContain('activationOnceHint')
    expect(userView).not.toContain('<el-switch :model-value="row.status')
  })

})
