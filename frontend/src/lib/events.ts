// The server-sent event stream, exposed as Svelte stores.
//
// One connection carries log lines, configuration changes and download
// progress, so a view subscribes to what it cares about rather than opening
// its own socket.
//
// It is read with fetch rather than EventSource, because EventSource cannot
// send an Authorization header and the stream needs the token like every
// other API call. The format is still plain SSE, which is what makes
// `curl /api/v1/events` show the same thing.

import { get, readable, writable, type Readable } from 'svelte/store'
import { authorizedFetch, type DownloadJob, type DownloadOutcome, type LogEntry } from './api'
import { signedIn } from './auth'

export const EVENT_LOG = 'log'
export const EVENT_CONFIG_CHANGED = 'config-changed'
export const EVENT_DOWNLOAD_STARTED = 'download-started'
export const EVENT_DOWNLOAD_FINISHED = 'download-finished'

/** How many log entries the live tail keeps in the page. */
const TAIL_LIMIT = 1000

/** How long to wait before reconnecting a dropped stream. */
const RECONNECT_MS = 3000

/** True while the stream to the daemon is connected. */
export const connected = writable(false)

/** The download in progress, wherever it was started, or null. */
export const downloading = writable<DownloadJob | null>(null)

/** The last download to finish. */
export const lastDownload = writable<DownloadOutcome | null>(null)

/** Increments on every configuration change, so a view can reload on it. */
export const configVersion = writable(0)

type Listener = (entry: LogEntry) => void
const logListeners = new Set<Listener>()

let controller: AbortController | null = null
let reconnectTimer: number | undefined

/**
 * connect opens the stream, reconnecting whenever it drops -- including
 * across a daemon restart, which comes back on the same port.
 */
export function connect(): void {
  if (controller || !get(signedIn)) return

  const current = new AbortController()
  controller = current
  void run(current).finally(() => {
    connected.set(false)
    if (controller === current) controller = null
    if (!current.signal.aborted) {
      window.clearTimeout(reconnectTimer)
      reconnectTimer = window.setTimeout(connect, RECONNECT_MS)
    }
  })
}

/** disconnect closes the stream, for signing out. */
export function disconnect(): void {
  window.clearTimeout(reconnectTimer)
  controller?.abort()
  controller = null
  connected.set(false)
}

async function run(current: AbortController): Promise<void> {
  let resp: Response
  try {
    resp = await authorizedFetch('/events', {
      headers: { Accept: 'text/event-stream' },
      signal: current.signal,
    })
  } catch {
    return
  }
  if (!resp.ok || !resp.body) return
  connected.set(true)

  const reader = resp.body.pipeThrough(new TextDecoderStream()).getReader()
  let buffer = ''
  try {
    for (;;) {
      const { value, done } = await reader.read()
      if (done) return
      buffer += value

      // Frames end with a blank line. Anything after the last one is a frame
      // still arriving.
      let end: number
      while ((end = buffer.indexOf('\n\n')) >= 0) {
        dispatch(buffer.slice(0, end))
        buffer = buffer.slice(end + 2)
      }
    }
  } catch {
    // Aborted, or the connection dropped; connect's finally reconnects.
  }
}

function dispatch(frame: string): void {
  let type = 'message'
  const data: string[] = []
  for (const line of frame.split('\n')) {
    if (line.startsWith(':')) continue // heartbeat comment
    if (line.startsWith('event:')) type = line.slice(6).trim()
    else if (line.startsWith('data:')) data.push(line.slice(5).trimStart())
  }
  if (data.length === 0) return

  let payload: unknown
  try {
    payload = JSON.parse(data.join('\n'))
  } catch {
    return
  }

  switch (type) {
    case EVENT_LOG:
      logListeners.forEach((fn) => fn(payload as LogEntry))
      break
    case EVENT_CONFIG_CHANGED:
      configVersion.update((n) => n + 1)
      break
    case EVENT_DOWNLOAD_STARTED:
      downloading.set(payload as DownloadJob)
      break
    case EVENT_DOWNLOAD_FINISHED:
      downloading.set(null)
      lastDownload.set(payload as DownloadOutcome)
      break
  }
}

/**
 * tail returns a store of live log entries, newest last.
 *
 * It is created per subscriber rather than being a single global, so a view
 * that stops following stops accumulating entries.
 */
export function tail(): Readable<LogEntry[]> {
  return readable<LogEntry[]>([], (set) => {
    let entries: LogEntry[] = []

    const listener: Listener = (entry) => {
      entries = [...entries, entry]
      // Bounded for the same reason the server's ring is: a debug-level
      // session would otherwise grow the page without limit.
      if (entries.length > TAIL_LIMIT) {
        entries = entries.slice(entries.length - TAIL_LIMIT)
      }
      set(entries)
    }

    logListeners.add(listener)
    connect()
    return () => logListeners.delete(listener)
  })
}
