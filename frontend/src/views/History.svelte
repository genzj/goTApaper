<script lang="ts">
  import {
    ApiError,
    api,
    type HistoryEntry,
    type HistoryPage,
    type SweepResult,
  } from '../lib/api'
  import AuthImage from '../lib/AuthImage.svelte'
  import Icon from '../lib/Icon.svelte'
  import PhotoLightbox, { type LightboxSpec } from '../lib/PhotoLightbox.svelte'
  import { bytes, stamp } from '../lib/format'
  import { downloading, lastDownload } from '../lib/events'

  const PAGE = 24

  let history = $state<HistoryPage | null>(null)
  let offset = $state(0)
  let error = $state('')
  let pruneResult = $state<SweepResult | null>(null)
  let inspected = $state<HistoryEntry | null>(null)
  let latestEntryId = $state<number | null>(null)
  let latestExists = $state(false)

  async function load() {
    try {
      const [pageData, last] = await Promise.all([
        api.history(PAGE, offset),
        api.lastApplied().catch(() => null),
      ])
      history = pageData
      latestEntryId = last?.entry?.id ?? null
      latestExists = Boolean(last?.exists)
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
      const updated = await api.loadPreview(entry.id)
      await load()
      if (inspected?.id === entry.id) {
        inspected = updated
      }
    } catch (e) {
      error =
        (e instanceof Error ? e.message : String(e)) +
        ' — the provider may no longer serve this picture.'
    }
    previewing[entry.id] = false
  }

  async function examine(entry: HistoryEntry) {
    if (!entry.hasThumbnail && entry.url) {
      await loadPreview(entry)
      const refreshed = history?.entries.find((e) => e.id === entry.id)
      if (refreshed?.hasThumbnail) {
        inspected = refreshed
      }
      return
    }
    if (entry.hasThumbnail) {
      inspected = entry
    }
  }

  function inspectPath(entry: HistoryEntry): string {
    if (entry.id === latestEntryId && latestExists) {
      return '/wallpaper/last-applied/image'
    }
    return `/history/${entry.id}/thumbnail`
  }

  function inspectBadges(entry: HistoryEntry): string[] {
    const out = [entry.channelName]
    if (entry.channelType) out.push(entry.channelType)
    if (entry.timesApplied > 1) out.push(`${entry.timesApplied}× applied`)
    return out
  }

  function inspectSpecs(entry: HistoryEntry): LightboxSpec[] {
    return [
      { label: 'Title', value: entry.title || '(untitled)' },
      { label: 'Credit', value: entry.credit || '—' },
      {
        label: 'Channel',
        value: entry.channelType
          ? `${entry.channelName} (${entry.channelType})`
          : entry.channelName,
      },
      { label: 'Last Applied', value: stamp(entry.downloadTime) },
      { label: 'First Applied', value: stamp(entry.firstAppliedTime) },
      { label: 'Times Applied', value: String(entry.timesApplied) },
      { label: 'Source URL', value: entry.url || '—', mono: true },
    ]
  }

  async function remove(entry: HistoryEntry) {
    error = ''
    try {
      await api.deleteHistoryEntry(entry.id)
      if (inspected?.id === entry.id) inspected = null
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

<div class="toolbar">
  <div class="toolbar-left">
    <strong>Wallpaper Gallery</strong>
    {#if history}
      <span class="badge">{history.total} recorded</span>
    {/if}
    {#if pruneResult}
      <span class="badge accent">
        {pruneResult.dryRun ? 'Would remove' : 'Removed'}
        {pruneResult.rowsRemoved} entries · {pruneResult.filesRemoved} thumbnails
        ({bytes(pruneResult.bytesReclaimed)})
      </span>
    {/if}
  </div>
  <div class="row">
    <button onclick={() => prune(true)}>
      <Icon name="sparkles" size={15} />
      <span>Preview cleanup</span>
    </button>
    <button onclick={() => prune(false)}>
      <Icon name="trash" size={15} />
      <span>Clean up now</span>
    </button>
  </div>
</div>

{#if error}
  <p class="banner">{error}</p>
{/if}

{#if history === null}
  <div class="empty-state">
    <p class="muted">Loading history gallery…</p>
  </div>
{:else if history.entries.length === 0}
  <div class="empty-state">
    <Icon name="history" size={28} />
    <strong>No wallpapers have been applied yet</strong>
    <p class="muted">Pictures applied by goTApaper will appear here with their thumbnails and metadata.</p>
  </div>
{:else}
  <div class="grid">
    {#each history.entries as entry (entry.id)}
      <article class="card item">
        <div class="media">
          {#if entry.hasThumbnail}
            <button
              type="button"
              class="thumb-trigger"
              onclick={() => (inspected = entry)}
              title="Click to enlarge and examine photo"
            >
              <AuthImage path={`/history/${entry.id}/thumbnail`} alt={entry.title ?? ''} />
              <span class="zoom-overlay">
                <Icon name="zoom-in" size={14} />
                <span>Examine</span>
              </span>
            </button>
          {:else}
            <div class="noimage muted">
              {#if entry.url}
                <button onclick={() => loadPreview(entry)} disabled={previewing[entry.id]}>
                  <Icon name="refresh" size={14} />
                  <span>{previewing[entry.id] ? 'Loading…' : 'Load preview'}</span>
                </button>
              {:else}
                <span>no preview</span>
              {/if}
            </div>
          {/if}
          <div class="media-badges">
            <span class="badge accent">{entry.channelName}</span>
            {#if entry.timesApplied > 1}
              <span class="badge">{entry.timesApplied}× applied</span>
            {/if}
          </div>
        </div>

        <div class="meta">
          <strong class="title">{entry.title || '(untitled)'}</strong>
          {#if entry.credit}
            <span class="credit muted">{entry.credit}</span>
          {/if}
          <div class="timestamps muted">
            <span>Last: {stamp(entry.downloadTime)}</span>
            {#if entry.firstAppliedTime && entry.firstAppliedTime !== entry.downloadTime}
              <span>First: {stamp(entry.firstAppliedTime)}</span>
            {/if}
          </div>
          {#if entry.url}
            <span class="mono muted break">{entry.url}</span>
          {/if}
        </div>

        <div class="row item-actions">
          <button
            class="reapply-btn"
            onclick={() => reapply(entry)}
            disabled={!entry.url || $downloading !== null}
          >
            <Icon name="refresh" size={14} />
            <span>Re-apply</span>
          </button>
          {#if entry.hasThumbnail || entry.url}
            <button
              type="button"
              onclick={() => examine(entry)}
              disabled={previewing[entry.id]}
              title="Enlarge and examine photo"
            >
              <Icon name="expand" size={14} />
              <span>Examine</span>
            </button>
          {/if}
          <button
            class="btn-danger delete-btn"
            onclick={() => remove(entry)}
            title="Delete history entry"
          >
            <Icon name="trash" size={14} />
            <span>Delete</span>
          </button>
        </div>
      </article>
    {/each}
  </div>

  <div class="toolbar pager">
    <span class="muted pager-count">
      Showing <strong>{offset + 1}–{offset + history.entries.length}</strong> of
      <strong>{history.total}</strong>
    </span>
    <div class="row">
      <button onclick={() => page(-1)} disabled={offset === 0}>
        <Icon name="chevron-left" size={15} />
        <span>Newer</span>
      </button>
      <button
        onclick={() => page(1)}
        disabled={offset + history.entries.length >= history.total}
      >
        <span>Older</span>
        <Icon name="chevron-right" size={15} />
      </button>
    </div>
  </div>

  {#if inspected}
    <PhotoLightbox
      path={inspectPath(inspected)}
      title={inspected.title || '(untitled)'}
      subtitle={inspected.credit || ''}
      caption={inspected.caption && inspected.caption !== inspected.title
        ? inspected.caption
        : ''}
      badges={inspectBadges(inspected)}
      specs={inspectSpecs(inspected)}
      canReapply={Boolean(inspected.url)}
      reapplyDisabled={!inspected.url || $downloading !== null}
      onReapply={() => inspected && reapply(inspected)}
      onClose={() => (inspected = null)}
    />
  {/if}
{/if}

<style>
  .toolbar-left {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .toolbar-left strong {
    font-size: var(--text-md);
    margin-right: var(--space-1);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(17.5rem, 1fr));
    gap: var(--space-4);
  }

  .item {
    display: flex;
    flex-direction: column;
    margin-bottom: 0;
    padding: var(--space-3);
    border-radius: var(--radius-lg);
  }

  .item:hover {
    border-color: var(--border-strong);
    box-shadow: var(--shadow-md);
  }

  .media {
    position: relative;
  }

  .thumb-trigger {
    position: relative;
    display: block;
    width: 100%;
    padding: 0;
    border: 1px solid var(--border-subtle);
    background: var(--surface-subtle);
    box-shadow: none;
    cursor: zoom-in;
    border-radius: var(--radius-md);
    overflow: hidden;
    transition:
      border-color var(--duration-normal) var(--ease-out),
      box-shadow var(--duration-normal) var(--ease-out);
  }

  .thumb-trigger:hover:not(:disabled),
  .thumb-trigger:focus-visible {
    background: var(--surface-subtle);
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-surface), var(--shadow-sm);
  }

  .thumb-trigger :global(img) {
    border: none;
    box-shadow: none;
  }

  .zoom-overlay {
    position: absolute;
    right: var(--space-2);
    bottom: var(--space-2);
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.2rem 0.55rem;
    border-radius: var(--radius-pill);
    background: var(--accent);
    border: 1px solid var(--accent);
    color: var(--accent-fg);
    font-size: 0.7rem;
    font-weight: 600;
    box-shadow: var(--shadow-sm);
    opacity: 0;
    transform: translateY(4px);
    transition:
      opacity var(--duration-fast) var(--ease-out),
      transform var(--duration-fast) var(--ease-out);
    pointer-events: none;
  }

  .thumb-trigger:hover .zoom-overlay,
  .thumb-trigger:focus-visible .zoom-overlay {
    opacity: 1;
    transform: translateY(0);
  }

  .media-badges {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin-top: var(--space-2);
  }

  .item :global(img),
  .item :global(.missing),
  .noimage {
    width: 100%;
    aspect-ratio: 16 / 9;
    object-fit: cover;
    border-radius: var(--radius-md);
    border: 1px solid var(--border-subtle);
    display: block;
    background: var(--surface-subtle);
  }

  .noimage {
    display: grid;
    place-items: center;
    font-size: var(--text-xs);
    font-weight: 500;
  }

  .meta {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    margin: var(--space-2) 0 var(--space-3);
    font-size: var(--text-xs);
    flex: 1;
  }

  .title {
    font-size: var(--text-base);
    color: var(--fg);
    line-height: 1.35;
  }

  .credit {
    line-height: 1.35;
  }

  .timestamps {
    display: flex;
    flex-direction: column;
    gap: 0.1rem;
    font-size: 0.725rem;
    margin-top: 0.1rem;
  }

  .break {
    overflow-wrap: anywhere;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
    overflow: hidden;
    margin-top: var(--space-1);
    font-size: 0.7rem;
  }

  .item-actions {
    margin-top: auto;
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .reapply-btn:not(:disabled) {
    border-color: var(--accent-border);
    background: var(--accent-surface);
    color: var(--accent);
    font-weight: 600;
  }

  .reapply-btn:hover:not(:disabled) {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-fg);
  }

  .delete-btn {
    margin-left: auto;
  }

  .pager {
    margin-top: var(--space-5);
    margin-bottom: 0;
  }

  .pager-count {
    font-size: var(--text-sm);
  }

  .pager-count strong {
    color: var(--fg);
  }

  @media (max-width: 480px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
