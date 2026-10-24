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
