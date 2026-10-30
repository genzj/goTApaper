// Small display helpers, shared by the views.

/** bytes formats a byte count, matching the CLI's util.HumanBytes output. */
export function bytes(n: number | undefined): string {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  let value = n
  for (const suffix of ['KiB', 'MiB', 'GiB', 'TiB']) {
    value /= 1024
    if (value < 1024) {
      return `${value.toFixed(1).replace(/\.0$/, '')} ${suffix}`
    }
  }
  return `${(value / 1024).toFixed(1)} PiB`
}

/** unixTime formats a Unix timestamp in the user's locale. */
export function unixTime(seconds: number | undefined): string {
  if (!seconds) return '—'
  return new Date(seconds * 1000).toLocaleString()
}

/** clock formats an ISO timestamp as a time of day, for log lines. */
export function clock(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleTimeString([], { hour12: false }) +
    '.' + String(date.getMilliseconds()).padStart(3, '0')
}

/**
 * mapToLines renders a string map as one "key=value" per line, the form a
 * stringMap option is edited in.
 */
export function mapToLines(value: unknown): string {
  if (!value || typeof value !== 'object') return ''
  return Object.entries(value as Record<string, unknown>)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

/** linesToMap parses mapToLines' form back, ignoring blank lines. */
export function linesToMap(text: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const eq = trimmed.indexOf('=')
    if (eq <= 0) continue
    out[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1).trim()
  }
  return out
}
