<script lang="ts">
  import { api, type LogEntry, type LogFile } from '../lib/api'
  import { bytes, clock, unixTime } from '../lib/format'
  import { tail } from '../lib/events'

  const LEVELS = ['', 'error', 'warning', 'info', 'debug'] as const

  let level = $state('')
  let lines = $state(200)
  let following = $state(false)
  let snapshot = $state<LogEntry[]>([])
  let files = $state<LogFile[]>([])
  let logError = $state('')

  const live = tail()

  /**
   * What the table shows: the snapshot, plus live entries when following.
   *
   * The two are merged on sequence number rather than concatenated, because a
   * live entry can arrive between the snapshot being requested and rendered.
   */
  const shown = $derived.by(() => {
    if (!following) return filtered(snapshot)

    const bySeq = new Map<number, LogEntry>()
    for (const e of snapshot) bySeq.set(e.seq, e)
    for (const e of $live) bySeq.set(e.seq, e)
    return filtered([...bySeq.values()].sort((a, b) => a.seq - b.seq))
  })

  const SEVERITY: Record<string, number> = {
    panic: 0, fatal: 1, error: 2, warn: 3, warning: 3, info: 4, debug: 5, trace: 6,
  }

  function filtered(entries: LogEntry[]): LogEntry[] {
    if (!level) return entries
    const want = SEVERITY[level] ?? 6
    return entries.filter((e) => (SEVERITY[e.level] ?? 6) <= want)
  }

  async function loadLogs() {
    try {
      const page = await api.logs(level, lines)
      snapshot = page.entries
      logError = ''
    } catch (e) {
      logError = e instanceof Error ? e.message : String(e)
    }
  }

  async function loadFiles() {
    try {
      files = await api.logFiles()
    } catch {
      // A missing log file is not worth an error banner; the list simply
      // stays empty.
      files = []
    }
  }

  function fields(entry: LogEntry): string {
    if (!entry.fields) return ''
    return Object.keys(entry.fields)
      .sort()
      .map((k) => `${k}=${entry.fields![k]}`)
      .join(' ')
  }

  $effect(() => {
    loadLogs()
    loadFiles()
  })
</script>

<div class="row">
  <label>
    Level
    <select bind:value={level} onchange={loadLogs}>
      {#each LEVELS as option (option)}
        <option value={option}>{option || 'all'}</option>
      {/each}
    </select>
  </label>
  <label>
    Lines
    <input type="number" min="10" max="2000" step="10" bind:value={lines} onchange={loadLogs} />
  </label>
  <label title="Append new entries as they are logged">
    <input type="checkbox" bind:checked={following} />
    Follow
  </label>
  <button onclick={loadLogs}>Reload</button>
</div>

{#if logError}
  <p class="banner">{logError}</p>
{/if}

<div class="logs" class:following>
  <table>
    <tbody>
      {#each shown as entry (entry.seq)}
        <tr class={entry.level}>
          <td class="mono time">{clock(entry.time)}</td>
          <td class="mono lvl">{entry.level}</td>
          <td class="mono msg">
            {entry.message}
            {#if entry.fields}<span class="muted"> {fields(entry)}</span>{/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
  {#if shown.length === 0}
    <p class="muted pad">Nothing logged yet.</p>
  {/if}
</div>

{#if files.length > 0}
  <h3>Log files</h3>
  <table>
    <thead>
      <tr><th>Name</th><th>Size</th><th>Modified</th><th>Path</th></tr>
    </thead>
    <tbody>
      {#each files as file (file.name)}
        <tr>
          <td>{file.name}{#if file.active} <span class="muted">(active)</span>{/if}</td>
          <td>{bytes(file.size)}</td>
          <td>{unixTime(file.modTime)}</td>
          <td class="mono muted">{file.path}</td>
        </tr>
      {/each}
    </tbody>
  </table>
{/if}

<style>
  .logs {
    border: 1px solid var(--border);
    border-radius: 6px;
    max-height: 22rem;
    overflow: auto;
  }

  .logs.following {
    /* Keeps the newest line in view while following. */
    display: flex;
    flex-direction: column-reverse;
  }

  .logs td {
    border-bottom: none;
    padding: 0.1rem 0.5rem;
    white-space: pre-wrap;
  }

  .time {
    width: 7rem;
    color: var(--muted);
  }

  .lvl {
    width: 5rem;
  }

  tr.error .lvl,
  tr.fatal .lvl,
  tr.panic .lvl {
    color: var(--error);
    font-weight: 600;
  }

  tr.warning .lvl,
  tr.warn .lvl {
    color: var(--warn);
  }

  tr.debug .time,
  tr.debug .lvl,
  tr.debug .msg {
    color: var(--muted);
  }

  .pad {
    padding: 0.5rem;
  }
</style>
