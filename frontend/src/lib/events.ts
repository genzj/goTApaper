// The server-sent event stream, exposed as Svelte stores.
//
// One connection carries log lines, configuration changes and refresh
// progress, so a view subscribes to what it cares about rather than opening
// its own socket.

import { readable, writable, type Readable } from 'svelte/store'
import { eventsURL, type LogEntry, type RefreshResult } from './api'

export const EVENT_LOG = 'log'
export const EVENT_CONFIG_CHANGED = 'config-changed'
export const EVENT_REFRESH_STARTED = 'refresh-started'
export const EVENT_REFRESH_FINISHED = 'refresh-finished'

/** How many log entries the live tail keeps in the page. */
const TAIL_LIMIT = 1000

/** True while the stream is connected. */
export const connected = writable(false)

/** True between a refresh starting and finishing, wherever it was started. */
export const refreshing = writable(false)

/** The last refresh result seen on the stream. */
export const lastRefresh = writable<RefreshResult | null>(null)

/** Increments on every configuration change, so a view can reload on it. */
export const configVersion = writable(0)

type Listener = (entry: LogEntry) => void
const logListeners = new Set<Listener>()

let source: EventSource | null = null
let reconnectTimer: number | undefined

/**
 * Opens the stream, reconnecting if it drops.
 *
 * EventSource reconnects on its own after a network error but gives up on an
 * HTTP error, which is what a restarted daemon on a new port looks like, so
 * the retry is explicit.
 */
export function connect(): void {
  if (source) return

  source = new EventSource(eventsURL(), { withCredentials: true })

  source.addEventListener('open', () => connected.set(true))

  source.addEventListener('error', () => {
    connected.set(false)
    source?.close()
    source = null
    window.clearTimeout(reconnectTimer)
    reconnectTimer = window.setTimeout(connect, 3000)
  })

  source.addEventListener(EVENT_LOG, (e) => {
    const entry = parse<LogEntry>(e)
    if (entry) logListeners.forEach((fn) => fn(entry))
  })

  source.addEventListener(EVENT_CONFIG_CHANGED, () => {
    configVersion.update((n) => n + 1)
  })

  source.addEventListener(EVENT_REFRESH_STARTED, () => refreshing.set(true))

  source.addEventListener(EVENT_REFRESH_FINISHED, (e) => {
    refreshing.set(false)
    const result = parse<RefreshResult>(e)
    if (result) lastRefresh.set(result)
  })
}

function parse<T>(event: Event): T | null {
  const data = (event as MessageEvent).data
  if (typeof data !== 'string') return null
  try {
    return JSON.parse(data) as T
  } catch {
    return null
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
