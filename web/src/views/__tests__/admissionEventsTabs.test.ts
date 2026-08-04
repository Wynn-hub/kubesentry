import { describe, expect, it } from 'vitest'
import routerSource from '../../router/index.ts?raw'
import { eventDetailTarget, resolveEventsTab } from '../admissionEventsTabs'

describe('admission events tab routing', () => {
  // The incident drawer's sample links pointed at '/admission-events', a path no
  // route declares, so every sample click dead-ended. Assert against the router
  // source instead of restating the name, which would pass either way.
  it('targets a route name the router actually declares', () => {
    const target = eventDetailTarget('42')
    expect(target).toEqual({ name: 'events', query: { tab: 'events', event: '42' } })
    expect(routerSource).toContain(`name: '${target.name}'`)
  })

  // Both views share one route, so a same-route push carrying ?tab=events must
  // move the tab; otherwise the event drawer opens over the health tab.
  it('defaults every value except health to the event list', () => {
    expect(resolveEventsTab('health')).toBe('health')
    expect(resolveEventsTab('events')).toBe('events')
    expect(resolveEventsTab(undefined)).toBe('events')
    expect(resolveEventsTab(['health'])).toBe('events')
  })
})
