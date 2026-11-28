import type { HttpClient } from './http'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** OrderStatus is one status of a workflow, with the name staff see and the one clients see. Its `phase` decides every rule and the badge color. */
export type OrderStatus = Schemas['OrderStatus']
/** StatusWorkflow is a preset list of statuses ("Lista de status") at one version. */
export type StatusWorkflow = Schemas['StatusWorkflow']
/** StatusWorkflowList is every preset a store can choose. */
export type StatusWorkflowList = Schemas['StatusWorkflowList']

/**
 * listStatusWorkflows calls GET /api/v1/admin/status-workflows: the presets
 * with their statuses in order, for the choice in Configurações. Needs
 * `settings:read`.
 */
export function listStatusWorkflows(client: HttpClient): Promise<StatusWorkflowList> {
  return client.get<StatusWorkflowList>('/api/v1/admin/status-workflows')
}
