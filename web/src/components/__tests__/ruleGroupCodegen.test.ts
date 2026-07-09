import { describe, expect, it } from 'vitest'
import {
  conditionToRego, escapeRegoString, ruleGroupsToRego, ruleGroupToRego,
  serializeRuleGroups, deserializeRuleGroups,
  type Condition, type RuleGroup,
} from '../ruleGroupCodegen'

describe('escapeRegoString', () => {
  it('quotes a plain string', () => {
    expect(escapeRegoString('hello')).toBe('"hello"')
  })
  it('escapes double quotes and backslashes', () => {
    expect(escapeRegoString('a"b\\c')).toBe('"a\\"b\\\\c"')
  })
  it('escapes newlines and tabs', () => {
    expect(escapeRegoString('a\nb\tc')).toBe('"a\\nb\\tc"')
  })
})

describe('conditionToRego', () => {
  const path = (...fields: string[]) => fields.map((field) => ({ field, isArray: false }))

  it('eq', () => {
    const c: Condition = { path: path('spec', 'hostNetwork'), operator: 'eq', value: true }
    expect(conditionToRego(c)).toBe('input.request.object.spec.hostNetwork == true')
  })
  it('neq uses the not-equals-or-missing idiom', () => {
    const c: Condition = {
      path: path('spec', 'automountServiceAccountToken'), operator: 'neq', value: false,
    }
    expect(conditionToRego(c)).toBe('not input.request.object.spec.automountServiceAccountToken == false')
  })
  it('exists', () => {
    const c: Condition = { path: path('spec', 'nodeName'), operator: 'exists' }
    expect(conditionToRego(c)).toBe('_ = input.request.object.spec.nodeName')
  })
  it('notExists', () => {
    const c: Condition = { path: path('spec', 'nodeName'), operator: 'notExists' }
    expect(conditionToRego(c)).toBe('not input.request.object.spec.nodeName')
  })
  it('in', () => {
    const c: Condition = { path: path('spec', 'priorityClassName'), operator: 'in', value: ['a', 'b'] }
    expect(conditionToRego(c)).toBe('input.request.object.spec.priorityClassName == ["a", "b"][_]')
  })
  it('notIn', () => {
    const c: Condition = { path: path('spec', 'priorityClassName'), operator: 'notIn', value: ['a', 'b'] }
    expect(conditionToRego(c)).toBe('not input.request.object.spec.priorityClassName == ["a", "b"][_]')
  })
  it('regex', () => {
    const c: Condition = { path: path('metadata', 'name'), operator: 'regex', value: '^prod-' }
    expect(conditionToRego(c)).toBe('regex.match("^prod-", input.request.object.metadata.name)')
  })
  it('numeric comparisons', () => {
    const base = { path: path('spec', 'replicas') }
    expect(conditionToRego({ ...base, operator: 'gt', value: 3 })).toBe('input.request.object.spec.replicas > 3')
    expect(conditionToRego({ ...base, operator: 'lt', value: 3 })).toBe('input.request.object.spec.replicas < 3')
    expect(conditionToRego({ ...base, operator: 'gte', value: 3 })).toBe('input.request.object.spec.replicas >= 3')
    expect(conditionToRego({ ...base, operator: 'lte', value: 3 })).toBe('input.request.object.spec.replicas <= 3')
  })
  it('between', () => {
    const c: Condition = { path: path('spec', 'replicas'), operator: 'between', value: [1, 5] }
    expect(conditionToRego(c)).toBe('input.request.object.spec.replicas >= 1; input.request.object.spec.replicas <= 5')
  })
  it('between rejects a non-numeric operand', () => {
    const c: Condition = {
      path: path('spec', 'replicas'),
      operator: 'between',
      value: ['0; count(input.request.object.spec.containers)', 5] as unknown as [number, number],
    }
    expect(() => conditionToRego(c)).toThrow()
  })
  it('isTrue / isFalse', () => {
    const base = { path: path('spec', 'hostNetwork') }
    expect(conditionToRego({ ...base, operator: 'isTrue' })).toBe('input.request.object.spec.hostNetwork == true')
    expect(conditionToRego({ ...base, operator: 'isFalse' })).toBe('input.request.object.spec.hostNetwork == false')
  })
  it('count comparisons', () => {
    const base = { path: path('spec', 'containers') }
    expect(conditionToRego({ ...base, operator: 'countEq', value: 1 })).toBe('count(input.request.object.spec.containers) == 1')
    expect(conditionToRego({ ...base, operator: 'countGt', value: 1 })).toBe('count(input.request.object.spec.containers) > 1')
    expect(conditionToRego({ ...base, operator: 'countLt', value: 5 })).toBe('count(input.request.object.spec.containers) < 5')
  })

  it('binds a fresh variable when the path crosses an array', () => {
    const c: Condition = {
      path: [
        { field: 'spec', isArray: false },
        { field: 'containers', isArray: true },
        { field: 'securityContext', isArray: false },
        { field: 'privileged', isArray: false },
      ],
      operator: 'eq',
      value: true,
    }
    expect(conditionToRego(c)).toBe(
      'c0 := input.request.object.spec.containers[_]; c0.securityContext.privileged == true',
    )
  })

  it('binds successive variables for multiple array crossings', () => {
    const c: Condition = {
      path: [
        { field: 'spec', isArray: false },
        { field: 'containers', isArray: true },
        { field: 'env', isArray: true },
        { field: 'name', isArray: false },
      ],
      operator: 'eq',
      value: 'SECRET',
    }
    expect(conditionToRego(c)).toBe(
      'c0 := input.request.object.spec.containers[_]; c1 := c0.env[_]; c1.name == "SECRET"',
    )
  })

  it('renders a map key access', () => {
    const c: Condition = {
      path: [
        { field: 'metadata', isArray: false },
        { field: 'labels', isArray: false, isMap: true, mapKey: 'env' },
      ],
      operator: 'eq',
      value: 'prod',
    }
    expect(conditionToRego(c)).toBe('input.request.object.metadata.labels["env"] == "prod"')
  })
})

describe('ruleGroupToRego / ruleGroupsToRego', () => {
  const group: RuleGroup = {
    conditions: [
      { path: [{ field: 'spec', isArray: false }, { field: 'hostNetwork', isArray: false }], operator: 'isTrue' },
    ],
    message: 'hostNetwork not allowed',
  }

  it('wraps one group in a deny block with a group anchor comment', () => {
    expect(ruleGroupToRego(group, 0)).toBe(
      '# group-0\ndeny[msg] {\n  input.request.object.spec.hostNetwork == true\n  msg := "hostNetwork not allowed"\n}',
    )
  })

  it('joins multiple groups (OR across groups) under one package header', () => {
    const rego = ruleGroupsToRego([group, { ...group, message: 'second' }])
    expect(rego).toContain('package kubesentry')
    expect(rego).toContain('# group-0')
    expect(rego).toContain('# group-1')
    expect(rego.match(/deny\[msg\]/g)).toHaveLength(2)
  })

  it('escapes quotes in the message', () => {
    const rego = ruleGroupToRego({ ...group, message: 'say "hi"' }, 0)
    expect(rego).toContain('msg := "say \\"hi\\""')
  })

  it('throws for an empty group list', () => {
    expect(() => ruleGroupsToRego([])).toThrow()
  })
})

describe('serializeRuleGroups / deserializeRuleGroups', () => {
  it('round-trips', () => {
    const groups: RuleGroup[] = [
      { targetResource: 'pods', conditions: [], message: 'm' },
    ]
    expect(deserializeRuleGroups(serializeRuleGroups(groups))).toEqual(groups)
  })

  it('returns null for invalid JSON instead of throwing', () => {
    expect(deserializeRuleGroups('not json')).toBeNull()
  })

  it('returns null for well-formed JSON that is not a rule group array', () => {
    expect(deserializeRuleGroups('{"foo":1}')).toBeNull()
  })
})
