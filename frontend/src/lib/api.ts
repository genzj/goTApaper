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

/**
 * toInstance strips a channel as it was shown back to the channel as it is
 * stored. The server rejects unknown fields -- so a misspelled one cannot
 * look like a successful save -- and effectiveOptions is read-only output.
 */
export function toInstance(view: ChannelView): ChannelInstance {
  const { effectiveOptions: _effective, ...instance } = view
  return instance
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

/**
 * WatermarkLayer is one text overlay. Every size and position is relative to
 * the picture: size is a percentage of its shorter side, x and y place the
 * text box's anchor as fractions of its width and height, and paddings and
 * effect sizes are in em (multiples of the font size).
 */
export interface WatermarkLayer {
  id: number
  sortOrder: number
  enabled: boolean
  name: string
  template: string
  font: string
  size: number
  color: string
  anchor: string
  x: number
  y: number
  alignment: string
  linespace: number
  background: {
    color: string
    hThroughout: boolean
    vThroughout: boolean
    padding: [number, number, number, number]
  }
  shadow: { color: string; dx: number; dy: number }
  outline: { color: string; width: number }
}

export interface FontInfo {
  id: string
  family: string
  style: string
  file: string
  path: string
}

/** An installed font the renderer cannot load, and why. */
export interface UnsupportedFont {
  family: string
  style: string
  file: string
  path: string
  reason: string
}

export interface WatermarkPreset {
  id: string
  name: string
  description: string
  layers: WatermarkLayer[]
}

export interface TemplateField {
  name: string
  type: 'text' | 'time'
  description: string
  example: string
  snippet: string
  current?: string
}

export interface TemplateFunc {
  name: string
  signature: string
  description: string
  example: string
  result: string
  snippet: string
}

export interface TemplateHelp {
  fields: TemplateField[]
  funcs: TemplateFunc[]
}

/** A rectangle as fractions of the picture's width and height. */
export interface FractionRect {
  x: number
  y: number
  w: number
  h: number
}

export interface LayerResult {
  index: number
  id: number
  enabled: boolean
  text: string
  textBounds: FractionRect
  boxBounds: FractionRect
  fontPx: number
  error?: { stage: 'template' | 'font'; message: string; line?: number; column?: number }
}

export interface PreviewBase {
  entryId?: number
  title?: string
  url?: string
  width: number
  height: number
  capturedAt: number
  stale: boolean
}

export interface PreviewResult {
  revision: number
  width: number
  height: number
  layers: LayerResult[]
  base: PreviewBase
}

export interface HistoryEntry {
  /** The thumbnail file is on disk, not merely recorded. */
  hasThumbnail: boolean
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
  kind: 'refresh' | 'reapply' | 'preview' | 'revert'
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
  /** The desktop shows a watermark preview rather than the real wallpaper. */
  previewing?: boolean
  previewRevision?: number
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
  signal?: AbortSignal,
): Promise<T> {
  const headers: Record<string, string> = {}
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  const resp = await authorizedFetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
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
  // Saves the whole list. The layers apply from the next refresh.
  replaceWatermarkLayers: (layers: WatermarkLayer[]) =>
    request<WatermarkLayer[]>('PUT', '/watermark-layers', layers),

  // Watermark editor
  // Listed once per daemon run; restarting it picks up newly installed fonts.
  fonts: () => request<FontInfo[]>('GET', '/fonts'),
  unsupportedFonts: () => request<UnsupportedFont[]>('GET', '/fonts/unsupported'),
  watermarkPresets: () => request<WatermarkPreset[]>('GET', '/watermark-presets'),
  templateHelp: () => request<TemplateHelp>('GET', '/watermark/template-fields'),
  previewBase: () => request<PreviewBase>('GET', '/watermark/preview/base'),
  capturePreview: () => request<PreviewBase>('POST', '/watermark/preview/capture'),
  renderPreview: (layers: WatermarkLayer[], debug: boolean, signal?: AbortSignal) =>
    request<PreviewResult>('POST', '/watermark/preview', { layers, debug }, signal),
  /** The rendered preview, scaled; fetch it with authorizedFetch. */
  previewImagePath: (width: number, revision: number) =>
    `/watermark/preview/image?w=${Math.round(width)}&rev=${revision}`,
  /** Renders layers small without changing the preview, for the preset gallery. */
  previewSample: async (layers: WatermarkLayer[], width: number): Promise<Blob> => {
    const resp = await authorizedFetch('/watermark/preview/sample', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ layers, width }),
    })
    if (!resp.ok) throw new ApiError(`sample failed with ${resp.status}`, resp.status)
    return resp.blob()
  },
  // Both answer 202 and run as a job; 409 while a download runs.
  showPreviewOnDesktop: () => request<DownloadStatus>('POST', '/watermark/preview/desktop'),
  revertPreview: () => request<DownloadStatus>('POST', '/watermark/preview/revert'),

  // Wallpaper
  lastApplied: () => request<LastApplied>('GET', '/wallpaper/last-applied'),
  status: () => request<Status>('GET', '/status'),
  // The daemon answers and then restarts, coming back on the same port; the
  // event stream reconnects by itself.
  restartDaemon: () => request<void>('POST', '/daemon/restart'),
  // Both start a download in the background and return at once; how it went
  // arrives as a download-finished event. A 409 means one is already running.
  refresh: (channels: string[] = [], force = false) =>
    request<DownloadStatus>('POST', '/refresh', { channels, force }),

  // History
  history: (limit: number, offset: number) =>
    request<HistoryPage>('GET', `/history?limit=${limit}&offset=${offset}`),
  reapply: (id: number) => request<DownloadStatus>('POST', `/history/${id}/reapply`),
  // Downloads the picture and saves its thumbnail without applying it.
  loadPreview: (id: number) => request<HistoryEntry>('POST', `/history/${id}/preview`),
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
