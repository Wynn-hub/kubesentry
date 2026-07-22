import { useRoute, useRouter, type RouteLocationRaw, type RouteParamsGeneric } from 'vue-router'

export interface BackFallback {
  name: string
  inheritParams?: string[]
  fallbackName?: string
}

declare module 'vue-router' {
  interface RouteMeta {
    backFallback?: BackFallback
  }
}

export function isValidApplicationBack(back: unknown, currentFullPath: string, base: string): back is string {
  if (typeof back !== 'string' || !back.startsWith('/') || back.startsWith('//')) return false

  const path = back.split(/[?#]/, 1)[0]
  const normalizedBase = base === '/' ? '' : `/${base.replace(/^\/+|\/+$/g, '')}`
  if (normalizedBase && path !== normalizedBase && !path.startsWith(`${normalizedBase}/`)) return false

  const applicationPath = back.slice(normalizedBase.length) || '/'
  return applicationPath !== currentFullPath
}

export function buildBackFallbackTarget(
  fallback: BackFallback | undefined,
  params: RouteParamsGeneric,
): RouteLocationRaw {
  if (!fallback) return { name: 'overview' }
  if (!fallback.inheritParams?.length) return { name: fallback.name }

  const inherited: RouteParamsGeneric = {}
  for (const key of fallback.inheritParams) {
    const value = params[key]
    if (value === undefined || value === null || value === '') {
      return { name: fallback.fallbackName ?? 'overview' }
    }
    inherited[key] = value
  }
  return { name: fallback.name, params: inherited }
}

export function useBackNavigation() {
  const route = useRoute()
  const router = useRouter()

  function goBack(): void {
    const historyBack = window.history.state?.back
    if (isValidApplicationBack(historyBack, route.fullPath, router.options.history.base)) {
      router.back()
      return
    }

    const fallback = route.meta.backFallback
    let target = buildBackFallbackTarget(fallback, route.params)
    try {
      router.resolve(target)
    } catch {
      target = { name: fallback?.fallbackName ?? 'overview' }
    }
    void router.push(target)
  }

  return { goBack }
}
