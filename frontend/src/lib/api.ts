// One typed wrapper over the REST API.
//
// The interface speaks the same HTTP API the CLI and curl do, rather than
// binding to Go methods through the webview bridge. That is deliberate: it
// means every capability the UI has is reachable from a terminal too, by
// construction rather than by discipline.
//
// Every request carries the bearer token from auth.ts. A 401 means the token
// is no longer valid -- the daemon was reinstalled, or its token file deleted
// -- so the interface signs out and asks again rather than showing a page of
// errors.

import { get } from 'svelte/store'
import { signOut, token } from './auth'

/** Stands in for a secret the server will not disclose. */
export const REDACTED = '********'

export type Kind = 'string' | 'int' | 'float' | 'bool' | 'enum' | 'stringMap'

export interface ValueSpec {
  key: string
  label?: string
  kind: Kind
  required?: boolean
  default?: string
  enumValues?: string[]
  description?: string
  secret?: boolean
}

export interface Setting extends ValueSpec {
  value: string
  isDefault: boolean
}

export interface ChannelInstance {
  id: number
  name: string
  type: string
  templateName?: string
  enabled: boolean
  probability: number
  sortOrder: number
  strategy?: string | null
  width?: number | null
  height?: number | null
  options: Record<string, unknown>
  optionsVersion?: number
}

export interface ChannelView extends ChannelInstance {
  effectiveOptions: Record<string, unknown>
}

export interface ChannelTypeInfo {
  name: string
  schema: { version: number; options: ValueSpec[] }
}

export interface ChannelTemplate {
  id: number
  name: string
  type?: string
  options: Record<string, unknown>
}

export interface TemplateView extends ChannelTemplate {
  usedBy: string[]
}

export interface WatermarkLayer {
  id: number
  sortOrder: number
  enabled: boolean
  font: string
  point: number
  color: string
  position: string
  hMargin: number
  vMargin: number
  linespace: number
  alignment: string
  template: string
  absolutePoint: boolean
  absolutePosition: boolean
  background: {
    color: string
    hThroughout: boolean
    vThroughout: boolean
    paddings: number[]
  }
}

export interface HistoryEntry {
  /** When the picture was first applied. Equal to downloadTime unless re-applied. */
  firstAppliedTime: number
  /** How many times the picture has been applied, re-applications included. */
  timesApplied: number
  id: number
  channelName: string
  channelType?: string
  url?: string
  title?: string
  caption?: string
  credit?: string
  format?: string
  uploadTime?: number
  downloadTime: number
  filePath?: string
  thumbPath?: string
}

export interface HistoryPage {
  entries: HistoryEntry[]
  total: number
  limit: number
  offset: number
}

/**
 * The wallpaper goTApaper applied most recently. Not necessarily what is on
 * the desktop now: the user may have changed it by hand since.
 */
export interface LastApplied {
  entry?: HistoryEntry
  path?: string
  size?: number
  width?: number
  height?: number
  exists: boolean
}

export interface LogEntry {
  seq: number
  time: string
  level: string
  message: string
  fields?: Record<string, string>
}

export interface LogPage {
  entries: LogEntry[]
  lastSeq: number
  capacity: number
}

export interface LogFile {
  name: string
  path: string
  size: number
  modTime: number
  active: boolean
}

export interface SweepResult {
  rowsRemoved: number
  filesRemoved: number
  bytesReclaimed: number
  dryRun?: boolean
}

export interface RefreshResult {
  channel: string
  type: string
  title?: string
  credit?: string
}

export type DownloadState = 'idle' | 'downloading'

export interface DownloadJob {
  kind: 'refresh' | 'reapply'
  channels?: string[]
  force?: boolean
  entryId?: number
  scheduled?: boolean
}

export interface DownloadOutcome {
  job: DownloadJob
  result: RefreshResult
  error?: string
  finishedAt: number
}

export interface DownloadStatus {
  state: DownloadState
  job?: DownloadJob
  startedAt?: number
  last?: DownloadOutcome
}

export interface Status {
  version?: string
  download: DownloadStatus
}

/** An error carrying the server's own message and any structured detail. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    /** Names the channels blocking a template delete. */
    readonly usedBy?: string[],
    /** Lists the acceptable values when one was rejected. */
    readonly expected?: string[],
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

/**
 * authorizedFetch is fetch with the bearer token, for responses that are not
 * JSON: images and the event stream. A 401 signs out.
 */
export async function authorizedFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  const current = get(token)
  if (current) headers.set('Authorization', `Bearer ${current}`)

  const resp = await fetch(`/api/v1${path}`, { ...init, headers })
  if (resp.status === 401) signOut()
  return resp
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  const resp = await authorizedFetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  })

  if (!resp.ok) {
    let message = `${method} ${path} failed with ${resp.status}`
    let usedBy: string[] | undefined
    let expected: string[] | undefined
    try {
      const payload = await resp.json()
      // The server's message already names the accepted values or the
      // blocking dependants, so it beats anything invented here.
      if (payload?.error) message = payload.error
      usedBy = payload?.usedBy
      expected = payload?.expected
    } catch {
      // Not JSON. Keep the status-based message.
    }
    throw new ApiError(message, resp.status, usedBy, expected)
  }

  if (resp.status === 204) return undefined as T
  const text = await resp.text()
  return (text ? JSON.parse(text) : undefined) as T
}

export const api = {
  // Settings
  settings: () => request<Setting[]>('GET', '/settings'),
  settingDefinitions: () => request<ValueSpec[]>('GET', '/setting-definitions'),
  setSetting: (key: string, value: string) =>
    request<void>('PUT', `/settings/${encodeURIComponent(key)}`, { value }),
  unsetSetting: (key: string) =>
    request<void>('DELETE', `/settings/${encodeURIComponent(key)}`),

  // Channels
  channels: () => request<ChannelView[]>('GET', '/channels'),
  channelTypes: () => request<ChannelTypeInfo[]>('GET', '/channel-types'),
  upsertChannel: (c: Partial<ChannelInstance>) =>
    request<ChannelView>('POST', '/channels', c),
  deleteChannel: (name: string) =>
    request<void>('DELETE', `/channels/${encodeURIComponent(name)}`),
  reorderChannels: (names: string[]) =>
    request<void>('POST', '/channels/reorder', { names }),

  // Channel templates
  channelTemplates: () => request<TemplateView[]>('GET', '/channel-templates'),
  upsertChannelTemplate: (t: Partial<ChannelTemplate>) =>
    request<TemplateView>('POST', '/channel-templates', t),
  deleteChannelTemplate: (name: string) =>
    request<void>('DELETE', `/channel-templates/${encodeURIComponent(name)}`),
  inlineChannelTemplate: (name: string) =>
    request<void>('POST', `/channel-templates/${encodeURIComponent(name)}/inline`),

  // Watermarks
  watermarkLayers: () => request<WatermarkLayer[]>('GET', '/watermark-layers'),
  upsertWatermarkLayer: (l: Partial<WatermarkLayer>) =>
    request<WatermarkLayer[]>('POST', '/watermark-layers', l),
  deleteWatermarkLayer: (id: number) =>
    request<void>('DELETE', `/watermark-layers/${id}`),

  // Wallpaper
  lastApplied: () => request<LastApplied>('GET', '/wallpaper/last-applied'),
  status: () => request<Status>('GET', '/status'),
  // Both start a download in the background and return at once; how it went
  // arrives as a download-finished event. A 409 means one is already running.
  refresh: (channels: string[] = [], force = false) =>
    request<DownloadStatus>('POST', '/refresh', { channels, force }),

  // History
  history: (limit: number, offset: number) =>
    request<HistoryPage>('GET', `/history?limit=${limit}&offset=${offset}`),
  reapply: (id: number) => request<DownloadStatus>('POST', `/history/${id}/reapply`),
  deleteHistoryEntry: (id: number) => request<void>('DELETE', `/history/${id}`),
  pruneHistory: (keepAtMost: number, dryRun: boolean) =>
    request<SweepResult>('POST', '/history/prune', { keepAtMost, dryRun }),

  // Logs
  logs: (level: string, limit: number) => {
    const query = new URLSearchParams()
    if (level) query.set('level', level)
    if (limit) query.set('limit', String(limit))
    return request<LogPage>('GET', `/logs?${query}`)
  },
  logFiles: () => request<LogFile[]>('GET', '/logs/files'),
}
