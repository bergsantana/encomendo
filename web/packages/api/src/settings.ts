import type { HttpClient } from './http'
import type { components } from './types/schema'

type Schemas = components['schemas']

/** TenantSettings is the selected store's settings: what is turned on, the deposit, the days, the time zone, the currency and the status workflow in use. */
export type TenantSettings = Schemas['TenantSettings']
/** EditableSettings are the settings the owner may change by hand. */
export type EditableSettings = Schemas['EditableSettings']
/** UpdateSettingsRequest is the body of PUT /api/v1/admin/settings: every editable field plus the `version` read. */
export type UpdateSettingsRequest = Schemas['UpdateSettingsRequest']
/** ChooseWorkflowRequest is the body of PUT /api/v1/admin/settings/status-workflow. */
export type ChooseWorkflowRequest = Schemas['ChooseWorkflowRequest']
/** TimeZone is one of the four Brazilian zones a store may use. */
export type TimeZone = Schemas['TimeZone']

/**
 * getSettings calls GET /api/v1/admin/settings. Every role may read it: the
 * apps hide Estoque and Produção when the store has them off.
 */
export function getSettings(client: HttpClient): Promise<TenantSettings> {
  return client.get<TenantSettings>('/api/v1/admin/settings')
}

/**
 * updateSettings calls PUT /api/v1/admin/settings (owner). Send every
 * editable field with the `version` that was read; someone saving first
 * throws `version_conflict`, and a value out of range throws `invalid` with
 * its field issues.
 */
export function updateSettings(client: HttpClient, body: UpdateSettingsRequest): Promise<TenantSettings> {
  return client.put<TenantSettings>('/api/v1/admin/settings', body)
}

/**
 * chooseStatusWorkflow calls PUT /api/v1/admin/settings/status-workflow
 * (owner). `version` is the preset's version and `expectedVersion` the
 * settings version read. New orders take the list; existing orders keep
 * theirs. Throws `workflow_not_found` and `version_conflict`.
 */
export function chooseStatusWorkflow(client: HttpClient, body: ChooseWorkflowRequest): Promise<TenantSettings> {
  return client.put<TenantSettings>('/api/v1/admin/settings/status-workflow', body)
}
