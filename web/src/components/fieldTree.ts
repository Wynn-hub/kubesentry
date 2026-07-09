import type { PathSegment } from './ruleGroupCodegen'

export interface FieldNode {
  name: string
  type: 'string' | 'integer' | 'number' | 'boolean' | 'array' | 'object'
  isArray: boolean
  isMap: boolean
  children?: FieldNode[]
}

export interface CascaderFieldOption {
  value: string
  label: string
  type: FieldNode['type']
  isArray: boolean
  isMap: boolean
  children?: CascaderFieldOption[]
}

// fieldTreeToCascaderOptions 把后端返回的字段树映射成 el-cascader 需要的
// { value, label, children } 形状，同时把 type/isArray/isMap 这些下游
// （算子选择、路径转 Rego）需要的元数据原样带着，挂在 option 对象上。
export function fieldTreeToCascaderOptions(nodes: FieldNode[]): CascaderFieldOption[] {
  return nodes.map((n) => ({
    value: n.name,
    label: n.name,
    type: n.type,
    isArray: n.isArray,
    isMap: n.isMap,
    // map 字段的 key 不可枚举，UI 走额外的文本输入收集 key，这里统一不下钻。
    children: n.isMap ? undefined : n.children && n.children.length > 0
      ? fieldTreeToCascaderOptions(n.children)
      : undefined,
  }))
}

function findNode(level: CascaderFieldOption[], value: string): CascaderFieldOption {
  const node = level.find((o) => o.value === value)
  if (!node) throw new Error(`unknown field path segment: ${value}`)
  return node
}

// cascaderPathToSegments 把 el-cascader 选中的 value 路径（字符串数组）
// 还原成携带 isArray/isMap 元数据的 PathSegment[]，供 codegen 使用。
export function cascaderPathToSegments(valuePath: string[], options: CascaderFieldOption[]): PathSegment[] {
  const segments: PathSegment[] = []
  let level = options
  for (const value of valuePath) {
    const node = findNode(level, value)
    segments.push({ field: node.value, isArray: node.isArray, isMap: node.isMap })
    level = node.children ?? []
  }
  return segments
}

// resolveLeafType 返回路径末端字段的类型，供算子下拉按类型过滤。
export function resolveLeafType(valuePath: string[], options: CascaderFieldOption[]): FieldNode['type'] {
  let level = options
  let node: CascaderFieldOption | undefined
  for (const value of valuePath) {
    node = findNode(level, value)
    level = node.children ?? []
  }
  if (!node) throw new Error('empty field path')
  return node.type
}

interface ResourceMatchEntry {
  apiGroups: string[]
  apiVersions: string[]
  resources: string[]
}

// findGroupVersionForResource 在 Policy 的 match.resources 里找到覆盖某个
// resource plural 名字的那一条，取其第一个 apiGroup/apiVersion 作为查询
// schema/fields 用的具体 GVK——一条 match 条目的 apiGroups/apiVersions 允许
// 填多个值，但 schema 端点一次只能按一个具体 GVK 查，第一个值就是 v1 的选择。
export function findGroupVersionForResource(
  resources: ResourceMatchEntry[],
  resourceName: string,
): { group: string; version: string } | null {
  for (const r of resources) {
    if (r.resources.includes(resourceName)) {
      return { group: r.apiGroups[0] ?? '', version: r.apiVersions[0] ?? 'v1' }
    }
  }
  return null
}
