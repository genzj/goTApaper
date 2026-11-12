<script lang="ts">
  import {
    ApiError,
    api,
    type HistoryEntry,
    type HistoryPage,
    type SweepResult,
  } from '../lib/api'
  import AuthImage from '../lib/AuthImage.svelte'
  import { bytes, stamp } from '../lib/format'
  import { downloading, lastDownload } from '../lib/events'

  const PAGE = 24

  let history = $state<HistoryPage | null>(null)
  let offset = $state(0)
  let error = $state('')
  let pruneResult = $state<SweepResult | null>(null)

  async function load() {
    try {
      history = await api.history(PAGE, offset)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function reapply(entry: HistoryEntry) {
    error = ''
    try {
      // Starts the download and returns; the outcome arrives as an event.
      await api.reapply(entry.id)
    } catch (e) {
      error =
        e instanceof ApiError && e.status === 409
          ? 'A download is already in progress.'
          : e instanceof Error
            ? e.message
            : String(e)
    }
  }

  // A failed re-apply is reported here, since the request itself only
  // started it. Several providers expire their links, so this is an expected
  // failure rather than a bug.
  $effect(() => {
    const done = $lastDownload
    if (done?.job.kind === 'reapply' && done.error) {
      error = `${done.error} — the provider may no longer serve this picture.`
    }
  })

  // Entries whose preview is being fetched, so each button shows its own
  // progress and cannot be clicked twice.
  let previewing = $state<Record<number, boolean>>({})

  async function loadPreview(entry: HistoryEntry) {
    error = ''
    previewing[entry.id] = true
    try {
      await api.loadPreview(entry.id)
      await load()
    } catch (e) {
      error =
        (e instanceof Error ? e.message : String(e)) +
        ' — the provider may no longer serve this picture.'
    }
    previewing[entry.id] = false
  }

  async function remove(entry: HistoryEntry) {
    error = ''
    try {
      await api.deleteHistoryEntry(entry.id)
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function prune(dryRun: boolean) {
    error = ''
    try {
      // 0 means "use the configured history.max-entries", so the button does
      // the same thing the automatic sweep does.
      pruneResult = await api.pruneHistory(0, dryRun)
      if (!dryRun) await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  function page(delta: number) {
    offset = Math.max(0, offset + delta * PAGE)
    load()
  }

  // Reload whenever a download finishes: a new picture, or a re-applied one
  // moving to the top.
  $effect(() => {
    void $lastDownload
    load()
  })
</script>

<div class="row">
  <button onclick={() => prune(true)}>Preview cleanup</button>
  <button onclick={() => prune(false)}>Clean up now</button>
  {#if pruneResult}
    <span class="muted">
      {pruneResult.dryRun ? 'Would remove' : 'Removed'}
      {pruneResult.rowsRemoved} entries and {pruneResult.filesRemoved} thumbnails
      ({bytes(pruneResult.bytesReclaimed)})
    </span>
  {/if}
</div>

{#if error}
  <p class="banner">{error}</p>
{/if}

{#if history === null}
  <p class="muted">Loading…</p>
{:else if history.entries.length === 0}
  <p class="muted">No wallpapers have been applied yet.</p>
{:else}
  <div class="grid">
    {#each history.entries as entry (entry.id)}
      <div class="card item">
        {#if entry.hasThumbnail}
          <AuthImage path={`/history/${entry.id}/thumbnail`} alt={entry.title ?? ''} />
        {:else}
          <div class="noimage muted">
            {#if entry.url}
              <button onclick={() => loadPreview(entry)} disabled={previewing[entry.id]}>
                {previewing[entry.id] ? 'Loading…' : 'Load preview'}
              </button>
            {:else}
              no preview
            {/if}
          </div>
        {/if}
        <div class="meta">
          <strong>{entry.title || '(untitled)'}</strong>
          <span class="muted">{entry.credit || ''}</span>
          <span class="muted">{entry.channelName}</span>
          <span class="muted">First applied {stamp(entry.firstAppliedTime)}</span>
          <span class="muted">
            Last applied {stamp(entry.downloadTime)}{#if entry.timesApplied > 1}
              · {entry.timesApplied} times{/if}
          </span>
          {#if entry.url}
            <span class="mono muted break">{entry.url}</span>
          {/if}
        </div>
        <div class="row">
          <button onclick={() => reapply(entry)} disabled={!entry.url || $downloading !== null}>
            Re-apply
          </button>
          <button onclick={() => remove(entry)}>Delete</button>
        </div>
      </div>
    {/each}
  </div>

  <div class="row pager">
    <button onclick={() => page(-1)} disabled={offset === 0}>Newer</button>
    <button
      onclick={() => page(1)}
      disabled={offset + history.entries.length >= history.total}>Older</button
    >
    <span class="muted">
      {offset + 1}–{offset + history.entries.length} of {history.total}
    </span>
  </div>
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: 0.75rem;
  }

  .item :global(img),
  .noimage {
    width: 100%;
    aspect-ratio: 16 / 9;
    object-fit: cover;
    border-radius: 4px;
    border: 1px solid var(--border);
    display: block;
  }

  .noimage {
    display: grid;
    place-items: center;
    font-size: 0.8rem;
  }

  .meta {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    margin: 0.4rem 0;
    font-size: 0.85rem;
  }

  .break {
    overflow-wrap: anywhere;
  }

  .pager {
    margin-top: 0.75rem;
  }
</style>
