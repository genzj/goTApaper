<script lang="ts">
  import { api, type LogEntry, type LogFile } from '../lib/api'
  import { bytes, clock, unixTime } from '../lib/format'
  import { tail } from '../lib/events'
  import Icon from '../lib/Icon.svelte'

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

<div class="toolbar">
  <div class="row filter-group">
    <label>
      <span>Level</span>
      <select bind:value={level} onchange={loadLogs}>
        {#each LEVELS as option (option)}
          <option value={option}>{option || 'all'}</option>
        {/each}
      </select>
    </label>
    <label>
      <span>Lines</span>
      <input
        type="number"
        min="10"
        max="2000"
        step="10"
        bind:value={lines}
        onchange={loadLogs}
      />
    </label>
    <label class="follow-toggle" title="Append new entries as they are logged">
      <input type="checkbox" bind:checked={following} />
      <span>Follow live</span>
    </label>
  </div>

  <div class="row">
    <span class="badge">{shown.length} entries</span>
    <button onclick={loadLogs}>
      <Icon name="reload" size={15} />
      <span>Reload</span>
    </button>
  </div>
</div>

{#if logError}
  <p class="banner">{logError}</p>
{/if}

<div class="console-card">
  <div class="console-header">
    <div class="console-title">
      <Icon name="logs" size={15} />
      <span>Daemon Event Stream</span>
    </div>
    {#if following}
      <span class="badge success">Live tail active</span>
    {:else}
      <span class="badge">Snapshot</span>
    {/if}
  </div>

  <div class="logs" class:following>
    <table>
      <tbody>
        {#each shown as entry (entry.seq)}
          <tr class={entry.level}>
            <td class="mono time">{clock(entry.time)}</td>
            <td class="mono lvl"><span class="lvl-pill">{entry.level}</span></td>
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
</div>

{#if files.length > 0}
  <section class="files-section">
    <div class="files-header">
      <h3>Log files</h3>
      <span class="badge">{files.length} files</span>
    </div>
    <div class="table-scroll">
      <table>
        <thead>
          <tr><th>Name</th><th>Size</th><th>Modified</th><th>Path</th></tr>
        </thead>
        <tbody>
          {#each files as file (file.name)}
            <tr>
              <td>
                <strong>{file.name}</strong>
                {#if file.active}
                  <span class="badge success">active</span>
                {/if}
              </td>
              <td>{bytes(file.size)}</td>
              <td>{unixTime(file.modTime)}</td>
              <td class="mono muted break">{file.path}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  </section>
{/if}

<style>
  .filter-group {
    gap: var(--space-4);
  }

  .filter-group label {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    font-weight: 500;
    color: var(--fg-secondary);
  }

  .filter-group input[type='number'] {
    width: 5.5rem;
  }

  .follow-toggle {
    padding: 0.25rem var(--space-3);
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background: var(--surface-subtle);
    cursor: pointer;
    min-height: 2.125rem;
  }

  .console-card {
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-sm);
    overflow: hidden;
  }

  .console-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-4);
    background: var(--surface-subtle);
    border-bottom: 1px solid var(--border);
  }

  .console-title {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted);
  }

  .logs {
    max-height: 30rem;
    overflow: auto;
  }

  .logs:focus-visible {
    outline: 2px solid var(--ring);
    outline-offset: -2px;
  }

  .logs.following {
    /* Keeps the newest line in view while following. */
    display: flex;
    flex-direction: column-reverse;
  }

  .logs table {
    border: none;
    border-radius: 0;
    box-shadow: none;
  }

  .logs tr:hover td {
    background: var(--surface-subtle);
  }

  .logs td {
    border-bottom: 1px solid var(--border-subtle);
    padding: 0.3rem var(--space-3);
    white-space: pre-wrap;
    font-size: var(--text-xs);
    line-height: 1.5;
    vertical-align: baseline;
  }

  .logs tr:last-child td {
    border-bottom: none;
  }

  .time {
    width: 7.25rem;
    color: var(--muted);
    white-space: nowrap;
  }

  .lvl {
    width: 5.5rem;
    white-space: nowrap;
  }

  .lvl-pill {
    display: inline-block;
    padding: 0.05rem 0.4rem;
    border-radius: 4px;
    font-size: 0.6875rem;
    text-transform: uppercase;
    font-weight: 600;
    letter-spacing: 0.03em;
    background: var(--surface-subtle);
    color: var(--fg-secondary);
  }

  tr.error td,
  tr.fatal td,
  tr.panic td {
    background: var(--error-surface);
  }

  tr.error .lvl-pill,
  tr.fatal .lvl-pill,
  tr.panic .lvl-pill {
    background: var(--error-surface);
    border: 1px solid var(--error-border);
    color: var(--error);
    font-weight: 700;
  }

  tr.warning .lvl-pill,
  tr.warn .lvl-pill {
    background: var(--warn-surface);
    border: 1px solid var(--warn-border);
    color: var(--warn);
  }

  tr.info .lvl-pill {
    background: var(--accent-surface);
    border: 1px solid var(--accent-border);
    color: var(--accent);
  }

  tr.debug .time,
  tr.debug .lvl-pill,
  tr.debug .msg {
    color: var(--muted);
  }

  .pad {
    padding: var(--space-5);
    margin: 0;
    text-align: center;
  }

  .files-section {
    margin-top: var(--space-6);
  }

  .files-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: var(--space-3);
  }

  .files-header h3 {
    margin: 0;
  }

  .break {
    overflow-wrap: anywhere;
  }
</style>
