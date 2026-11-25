/**
 * Order phases are fixed in the backend; each store names its own statuses
 * on top of them (docs/adr/0004-status-workflows.md). The UI never looks at
 * a status label to decide anything: the badge color comes from the phase.
 */
export const phases = [
  'draft',
  'awaiting_deposit',
  'confirmed',
  'sourcing',
  'assembling',
  'ready',
  'handed_over',
  'cancelled',
] as const

export type Phase = (typeof phases)[number]

/** A status color family from the design system. */
export type StatusTone = 'planned' | 'waiting' | 'assembly' | 'ready' | 'delivered' | 'neutral'

const toneByPhase: Record<Phase, StatusTone> = {
  draft: 'planned',
  awaiting_deposit: 'planned',
  confirmed: 'planned',
  sourcing: 'waiting',
  assembling: 'assembly',
  ready: 'ready',
  handed_over: 'delivered',
  cancelled: 'neutral',
}

/** toneForPhase returns the badge color family for an order phase. */
export function toneForPhase(phase: Phase): StatusTone {
  return toneByPhase[phase]
}

/** isPhase narrows a value from the API to a known phase. */
export function isPhase(value: string): value is Phase {
  return (phases as readonly string[]).includes(value)
}
