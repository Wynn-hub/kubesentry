export type AdmissionEventsTab = 'events' | 'health'

/** Events is the default: an absent, unknown, or cleared ?tab= lands on the list. */
export function resolveEventsTab(tab: unknown): AdmissionEventsTab {
  return tab === 'health' ? 'health' : 'events'
}

/**
 * Target for a link-incident sample event. Uses the route name rather than a
 * literal path — a hardcoded '/admission-events' silently resolved to nothing,
 * because the route this view lives on is named 'events' at path '/events'.
 */
export function eventDetailTarget(id: string) {
  return { name: 'events', query: { tab: 'events', event: id } } as const
}
