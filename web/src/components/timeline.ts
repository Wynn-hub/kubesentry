export interface TimelineState {
  cursor: number
  prevEnabled: boolean
  nextEnabled: boolean
  inFlight: boolean
}

// 相邻回滚约束的唯一判定点：按钮只渲染在游标的相邻节点上。
export function rollbackActionFor(version: number, tl: TimelineState): 'prev' | 'next' | null {
  if (tl.inFlight) return null
  if (tl.prevEnabled && version === tl.cursor - 1) return 'prev'
  if (tl.nextEnabled && version === tl.cursor + 1) return 'next'
  return null
}

export function pageForCurrentVersion(
  versions: readonly { version: number; isCurrent: boolean }[],
  currentVersion: number,
  pageSize: number,
  sortOrder: 'asc' | 'desc',
): number {
  const ordered = [...versions].sort((a, b) =>
    sortOrder === 'desc' ? b.version - a.version : a.version - b.version,
  )
  const index = ordered.findIndex((entry) => entry.isCurrent || entry.version === currentVersion)
  return index < 0 ? 1 : Math.floor(index / pageSize) + 1
}
