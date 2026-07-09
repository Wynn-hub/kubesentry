export type FieldLeafType = 'string' | 'integer' | 'number' | 'boolean' | 'array' | 'object'

export interface PathSegment {
  field: string
  isArray: boolean
  isMap?: boolean
  mapKey?: string
}

export type Operator =
  | 'eq' | 'neq' | 'exists' | 'notExists'
  | 'in' | 'notIn' | 'regex'
  | 'gt' | 'lt' | 'gte' | 'lte' | 'between'
  | 'isTrue' | 'isFalse'
  | 'countEq' | 'countGt' | 'countLt'

export interface Condition {
  path: PathSegment[]
  operator: Operator
  value?: string | number | boolean | string[] | [number, number]
}

export interface RuleGroup {
  targetResource?: string
  conditions: Condition[]
  message: string
}

export const VISUAL_BUILDER_ANNOTATION = 'kubesentry.io/visual-builder-spec'

// escapeRegoString 把任意字符串转成可以安全嵌入 Rego 源码的双引号字面量
// （含引号），处理反斜杠、双引号、换行等会破坏字符串边界的字符——这是防止
// 用户填的 value/message 拼出超出预期 Rego 逻辑的第一道防线。
export function escapeRegoString(raw: string): string {
  const escaped = raw
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/\n/g, '\\n')
    .replace(/\r/g, '\\r')
    .replace(/\t/g, '\\t')
  return `"${escaped}"`
}

function arrayLiteral(values: string[]): string {
  return `[${values.map(escapeRegoString).join(', ')}]`
}

function formatNumber(value: Condition['value']): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`expected a finite number, got ${JSON.stringify(value)}`)
  }
  return String(value)
}

function formatScalar(value: Condition['value']): string {
  if (typeof value === 'boolean') return value ? 'true' : 'false'
  if (typeof value === 'number') return formatNumber(value)
  return escapeRegoString(String(value ?? ''))
}

interface PathBuildResult {
  bindings: string[]
  expr: string
}

// buildPathExpr 把结构化字段路径拼成 Rego 表达式：普通字段直接拼 `.field`，
// 数组段引入一个新的局部变量做存在量词绑定（`cN := 当前表达式[_]`），后续
// 路径从该变量继续拼；map 段拼 `["key"]`。
function buildPathExpr(path: PathSegment[]): PathBuildResult {
  const bindings: string[] = []
  let expr = 'input.request.object'
  let arrayCount = 0
  for (const seg of path) {
    expr += `.${seg.field}`
    if (seg.isArray) {
      const varName = `c${arrayCount}`
      arrayCount += 1
      bindings.push(`${varName} := ${expr}[_]`)
      expr = varName
    }
    if (seg.isMap) {
      expr += `[${escapeRegoString(seg.mapKey ?? '')}]`
    }
  }
  return { bindings, expr }
}

export function conditionToRego(condition: Condition): string {
  const { bindings, expr } = buildPathExpr(condition.path)
  const prefix = bindings.length > 0 ? `${bindings.join('; ')}; ` : ''
  const v = condition.value

  switch (condition.operator) {
    case 'eq':
      return `${prefix}${expr} == ${formatScalar(v)}`
    case 'neq':
      return `${prefix}not ${expr} == ${formatScalar(v)}`
    case 'exists':
      return `${prefix}_ = ${expr}`
    case 'notExists':
      return `${prefix}not ${expr}`
    case 'in':
      return `${prefix}${expr} == ${arrayLiteral(v as string[])}[_]`
    case 'notIn':
      return `${prefix}not ${expr} == ${arrayLiteral(v as string[])}[_]`
    case 'regex':
      return `${prefix}regex.match(${escapeRegoString(v as string)}, ${expr})`
    case 'gt':
      return `${prefix}${expr} > ${formatNumber(v)}`
    case 'lt':
      return `${prefix}${expr} < ${formatNumber(v)}`
    case 'gte':
      return `${prefix}${expr} >= ${formatNumber(v)}`
    case 'lte':
      return `${prefix}${expr} <= ${formatNumber(v)}`
    case 'between': {
      const [low, high] = v as [number, number]
      return `${prefix}${expr} >= ${low}; ${expr} <= ${high}`
    }
    case 'isTrue':
      return `${prefix}${expr} == true`
    case 'isFalse':
      return `${prefix}${expr} == false`
    case 'countEq':
      return `${prefix}count(${expr}) == ${formatNumber(v)}`
    case 'countGt':
      return `${prefix}count(${expr}) > ${formatNumber(v)}`
    case 'countLt':
      return `${prefix}count(${expr}) < ${formatNumber(v)}`
    default:
      throw new Error(`unsupported operator: ${condition.operator satisfies never}`)
  }
}

export function ruleGroupToRego(group: RuleGroup, index: number): string {
  if (group.conditions.length === 0) {
    throw new Error(`group ${index} has no conditions`)
  }
  const body = group.conditions.map(conditionToRego).join('\n  ')
  return `# group-${index}\ndeny[msg] {\n  ${body}\n  msg := ${escapeRegoString(group.message)}\n}`
}

export function ruleGroupsToRego(groups: RuleGroup[]): string {
  if (groups.length === 0) {
    throw new Error('at least one rule group is required')
  }
  const blocks = groups.map((g, i) => ruleGroupToRego(g, i))
  return `package kubesentry\n\n${blocks.join('\n\n')}\n`
}

// serializeRuleGroups/deserializeRuleGroups persist the editor's structured
// state into the Policy's VISUAL_BUILDER_ANNOTATION — this is NOT parsing
// hand-written rego, just round-tripping the visual editor's own state so a
// policy built visually can be re-opened in visual mode later.
export function serializeRuleGroups(groups: RuleGroup[]): string {
  return JSON.stringify(groups)
}

export function deserializeRuleGroups(raw: string): RuleGroup[] | null {
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return null
    for (const g of parsed) {
      if (typeof g !== 'object' || g === null || !Array.isArray((g as RuleGroup).conditions)) {
        return null
      }
    }
    return parsed as RuleGroup[]
  } catch {
    return null
  }
}
