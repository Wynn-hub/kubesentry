export interface PolicyRefOut {
  name: string
  enforcementMode?: string
}

export interface ComputeMembershipInput {
  initialResolvedNames: string[]
  initialByNameNames: string[]
  initialExclude: string[]
  finalSelectedNames: string[]
  modeOverrides: Record<string, string | undefined>
}

export interface ComputeMembershipResult {
  byName: PolicyRefOut[]
  exclude: string[]
}

// computeMembership 把"当前实际生效成员 vs 用户最终勾选"的差异翻译成
// byName/exclude 的增删，让 console 用户只需要勾选/取消勾选，感知不到
// byName/bySelector/exclude 三种底层机制的区别。
//
// 规则（详见 docs/superpowers/specs/2026-07-08-policygroup-exclude-design.md §3.2）：
// - 取消勾选的成员：无论来源，都从 byName 摘除（若有）+ 加入 exclude
// - 新勾选的成员（相对初始生效集合是新增）：加入 byName，并从 exclude 摘除（若有）
// - 未变更的 byName 来源成员：保留在 byName
// - 未变更的 bySelector 来源成员：默认既不进 byName 也不进 exclude，
//   除非用户为其设置了显式模式覆盖（视为"钉入"意图）
export function computeMembership(input: ComputeMembershipInput): ComputeMembershipResult {
  const resolvedSet = new Set(input.initialResolvedNames)
  const byNameSet = new Set(input.initialByNameNames)
  const finalSet = new Set(input.finalSelectedNames)

  const byName: PolicyRefOut[] = []
  for (const name of input.finalSelectedNames) {
    const wasByName = byNameSet.has(name)
    const isNewlyAdded = !resolvedSet.has(name)
    const override = input.modeOverrides[name]
    const hasOverride = override !== undefined && override !== ''
    if (wasByName || isNewlyAdded || hasOverride) {
      byName.push({ name, enforcementMode: override || undefined })
    }
  }

  const removedNames = input.initialResolvedNames.filter((n) => !finalSet.has(n))
  const carriedExclude = input.initialExclude.filter((n) => !finalSet.has(n))
  const exclude = Array.from(new Set([...carriedExclude, ...removedNames]))

  return { byName, exclude }
}
