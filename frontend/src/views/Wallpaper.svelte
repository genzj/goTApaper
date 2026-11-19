<script lang="ts">
  import { ApiError, api, type LastApplied } from '../lib/api'
  import AuthImage from '../lib/AuthImage.svelte'
  import Icon from '../lib/Icon.svelte'
  import PhotoLightbox, { type LightboxSpec } from '../lib/PhotoLightbox.svelte'
  import { bytes, unixTime } from '../lib/format'
  import { configVersion, downloading, lastDownload } from '../lib/events'

  let last = $state<LastApplied | null>(null)
  let error = $state('')
  // Bumped to refetch the image after a download: the path is the same but
  // the picture behind it is not.
  let imageVersion = $state(0)
  let inspectOpen = $state(false)

  async function load() {
    try {
      last = await api.lastApplied()
      imageVersion++
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function refreshNow(force: boolean) {
    error = ''
    try {
      // Returns as soon as the download starts; the view reloads when the
      // download-finished event arrives.
      await api.refresh([], force)
    } catch (e) {
      error =
        e instanceof ApiError && e.status === 409
          ? 'A download is already in progress.'
          : e instanceof Error
            ? e.message
            : String(e)
    }
  }

  // Reload whenever configuration changes or a download finishes anywhere,
  // including one started from the tray or the CLI.
  $effect(() => {
    void $configVersion
    void $lastDownload
    load()
  })

  const failed = $derived($lastDownload?.error ?? '')

  const lightboxBadges = $derived.by(() => {
    if (!last?.entry) return []
    const out: string[] = [last.entry.channelName]
    if (last.entry.channelType) out.push(last.entry.channelType)
    if (last.width) out.push(`${last.width} × ${last.height}`)
    if (last.size) out.push(bytes(last.size))
    return out
  })

  const lightboxSpecs = $derived.by((): LightboxSpec[] => {
    if (!last?.entry) return []
    return [
      { label: 'Title', value: last.entry.title || '—' },
      { label: 'Credit', value: last.entry.credit || '—' },
      {
        label: 'Channel',
        value: last.entry.channelType
          ? `${last.entry.channelName} (${last.entry.channelType})`
          : last.entry.channelName,
      },
      {
        label: 'Dimensions',
        value: last.width ? `${last.width} × ${last.height}` : '—',
      },
      { label: 'Size', value: bytes(last.size) },
      { label: 'Applied', value: unixTime(last.entry.downloadTime) },
      { label: 'Uploaded', value: unixTime(last.entry.uploadTime) },
      { label: 'Local File', value: last.path || '—', mono: true },
      { label: 'Source URL', value: last.entry.url || '—', mono: true },
    ]
  })
</script>

<div class="toolbar">
  <div class="toolbar-info">
    <strong>Last applied wallpaper</strong>
    <span class="muted">
      What goTApaper set most recently. If the wallpaper was changed by hand since,
      the desktop shows something else.
    </span>
  </div>
  <div class="row">
    <button
      class="btn-primary"
      onclick={() => refreshNow(false)}
      disabled={$downloading !== null}
    >
      <Icon name="refresh" size={15} />
      <span>{$downloading ? 'Downloading…' : 'Refresh now'}</span>
    </button>
    <button
      onclick={() => refreshNow(true)}
      disabled={$downloading !== null}
      title="Download even if the channel would serve the same picture again"
    >
      <Icon name="bolt" size={15} />
      <span>Force refresh</span>
    </button>
    <button onclick={load} title="Reload metadata and preview">
      <Icon name="reload" size={15} />
      <span>Reload</span>
    </button>
  </div>
</div>

{#if error}
  <p class="banner">{error}</p>
{:else if failed}
  <p class="banner">The last download failed: {failed}</p>
{/if}

{#if last === null}
  <div class="empty-state">
    <p class="muted">Loading current wallpaper…</p>
  </div>
{:else if !last.entry}
  <div class="empty-state">
    <Icon name="wallpaper" size={28} />
    <strong>No wallpaper has been applied yet</strong>
    <p class="muted">
      Use <strong>Refresh now</strong> above to download and apply your first desktop wallpaper.
    </p>
  </div>
{:else}
  <div class="showcase">
    <section class="hero-stage-card">
      <div class="stage-viewport">
        {#if last.exists}
          <button
            type="button"
            class="photo-trigger"
            onclick={() => (inspectOpen = true)}
            title="Click to inspect full-resolution photo details"
          >
            <AuthImage
              path="/wallpaper/last-applied/image"
              alt={last.entry.title ?? 'Last applied wallpaper'}
              version={imageVersion}
            />
            <span class="inspect-pill">
              <Icon name="expand" size={14} />
              <span>Inspect full resolution</span>
            </span>
          </button>
        {:else}
          <div class="missing-file">
            <Icon name="wallpaper" size={24} />
            <p class="muted">
              The wallpaper file is no longer on disk, so there is nothing to preview.
            </p>
          </div>
        {/if}
      </div>

      <div class="hero-bar">
        <div class="hero-title-group">
          <div class="hero-badges">
            <span class="badge accent">{last.entry.channelName}</span>
            {#if last.entry.channelType}
              <span class="badge">{last.entry.channelType}</span>
            {/if}
            {#if last.width}
              <span class="badge">{last.width} × {last.height}</span>
            {/if}
            {#if last.size}
              <span class="badge">{bytes(last.size)}</span>
            {/if}
          </div>
          <h2>{last.entry.title || 'Untitled wallpaper'}</h2>
          {#if last.entry.caption && last.entry.caption !== last.entry.title}
            <p class="caption">{last.entry.caption}</p>
          {/if}
          {#if last.entry.credit}
            <p class="credit muted">{last.entry.credit}</p>
          {/if}
        </div>

        {#if last.exists}
          <button
            type="button"
            class="inspect-btn"
            onclick={() => (inspectOpen = true)}
          >
            <Icon name="zoom-in" size={15} />
            <span>Examine photo</span>
          </button>
        {/if}
      </div>
    </section>

    <section class="details-deck">
      <div class="stat-strip">
        <div class="stat-tile">
          <span class="stat-label">Dimensions</span>
          <strong class="stat-value">
            {last.width ? `${last.width} × ${last.height}` : '—'}
          </strong>
        </div>
        <div class="stat-tile">
          <span class="stat-label">File Size</span>
          <strong class="stat-value">{bytes(last.size)}</strong>
        </div>
        <div class="stat-tile">
          <span class="stat-label">Applied</span>
          <strong class="stat-value">{unixTime(last.entry.downloadTime)}</strong>
        </div>
        <div class="stat-tile">
          <span class="stat-label">Uploaded</span>
          <strong class="stat-value">{unixTime(last.entry.uploadTime)}</strong>
        </div>
      </div>

      <div class="meta-card">
        <h3>Picture Details</h3>
        <dl class="spec-grid">
          <div class="spec-row">
            <dt>Title</dt>
            <dd>{last.entry.title || '—'}</dd>
          </div>
          <div class="spec-row">
            <dt>Caption</dt>
            <dd>{last.entry.caption || '—'}</dd>
          </div>
          <div class="spec-row">
            <dt>Credit</dt>
            <dd>{last.entry.credit || '—'}</dd>
          </div>
          <div class="spec-row">
            <dt>Channel</dt>
            <dd>
              {last.entry.channelName}
              {#if last.entry.channelType}
                <span class="muted">({last.entry.channelType})</span>
              {/if}
            </dd>
          </div>
          <div class="spec-row">
            <dt>Local File</dt>
            <dd class="mono break">{last.path || '—'}</dd>
          </div>
          <div class="spec-row">
            <dt>Source URL</dt>
            <dd class="mono break">{last.entry.url || '—'}</dd>
          </div>
        </dl>
      </div>
    </section>
  </div>

  {#if inspectOpen && last.exists}
    <PhotoLightbox
      path="/wallpaper/last-applied/image"
      version={imageVersion}
      title={last.entry.title || 'Last applied wallpaper'}
      subtitle={last.entry.credit || ''}
      caption={last.entry.caption && last.entry.caption !== last.entry.title
        ? last.entry.caption
        : ''}
      badges={lightboxBadges}
      specs={lightboxSpecs}
      onClose={() => (inspectOpen = false)}
    />
  {/if}
{/if}

<style>
  .toolbar-info {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    max-width: 38rem;
  }

  .toolbar-info strong {
    font-size: var(--text-md);
    color: var(--fg);
  }

  .toolbar-info span {
    font-size: var(--text-xs);
    line-height: 1.4;
  }

  .showcase {
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }

  .hero-stage-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-3);
    box-shadow: var(--shadow-md);
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .stage-viewport {
    position: relative;
    width: 100%;
    border-radius: var(--radius-md);
    overflow: hidden;
    background: var(--surface-subtle);
  }

  .photo-trigger {
    position: relative;
    display: block;
    width: 100%;
    padding: 0;
    border: 1px solid var(--border);
    background: var(--surface-subtle);
    border-radius: var(--radius-md);
    cursor: zoom-in;
    overflow: hidden;
    box-shadow: var(--shadow-xs);
    transition:
      border-color var(--duration-normal) var(--ease-out),
      box-shadow var(--duration-normal) var(--ease-out);
  }

  .photo-trigger:hover:not(:disabled),
  .photo-trigger:focus-visible {
    background: var(--surface-subtle);
    border-color: var(--accent);
    box-shadow: 0 0 0 3px var(--accent-surface), var(--shadow-md);
  }

  .photo-trigger :global(img) {
    width: 100%;
    max-height: 68dvh;
    object-fit: contain;
    background: var(--surface-subtle);
    border: none;
    border-radius: var(--radius-md);
    box-shadow: none;
  }

  .inspect-pill {
    position: absolute;
    right: var(--space-3);
    bottom: var(--space-3);
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-pill);
    background: var(--surface);
    border: 1px solid var(--border);
    color: var(--fg);
    font-size: var(--text-xs);
    font-weight: 600;
    box-shadow: var(--shadow-sm);
    pointer-events: none;
    transition:
      transform var(--duration-fast) var(--ease-out),
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out),
      box-shadow var(--duration-fast) var(--ease-out);
  }

  .photo-trigger:hover .inspect-pill,
  .photo-trigger:focus-visible .inspect-pill {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-fg);
    transform: translateY(-2px);
    box-shadow: var(--shadow-md);
  }

  .missing-file {
    min-height: 22rem;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    padding: var(--space-6);
    background: var(--surface-subtle);
    border: 1px dashed var(--border);
    border-radius: var(--radius-md);
    text-align: center;
    color: var(--muted);
  }

  .missing-file p {
    margin: 0;
    font-size: var(--text-sm);
  }

  .hero-bar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-4);
    padding: 0 var(--space-2) var(--space-1);
  }

  .hero-title-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
    flex: 1;
  }

  .hero-badges {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .hero-title-group h2 {
    margin: 0;
    font-size: var(--text-xl);
    line-height: 1.3;
  }

  .caption {
    margin: 0;
    color: var(--fg-secondary);
    font-size: var(--text-base);
    line-height: 1.5;
    max-width: 58rem;
  }

  .credit {
    margin: 0;
    font-size: var(--text-xs);
  }

  .inspect-btn {
    align-self: center;
    flex-shrink: 0;
  }

  .details-deck {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  .stat-strip {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .stat-tile {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    padding: var(--space-3) var(--space-4);
    box-shadow: var(--shadow-xs);
    display: flex;
    flex-direction: column;
    gap: 0.2rem;
  }

  .stat-label {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--muted);
  }

  .stat-value {
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--fg);
    line-height: 1.3;
  }

  .meta-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    padding: var(--space-4);
    box-shadow: var(--shadow-xs);
  }

  .meta-card h3 {
    margin: 0 0 var(--space-3);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--muted);
  }

  .spec-grid {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 0 var(--space-6);
  }

  .spec-row {
    display: grid;
    grid-template-columns: 6.5rem 1fr;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-top: 1px solid var(--border-subtle);
    font-size: var(--text-sm);
  }

  .spec-row:nth-child(-n + 2) {
    border-top: none;
    padding-top: 0;
  }

  .spec-row dt {
    color: var(--muted);
    font-weight: 500;
  }

  .spec-row dd {
    margin: 0;
    color: var(--fg);
    min-width: 0;
  }

  .break {
    overflow-wrap: anywhere;
    word-break: break-word;
  }

  @media (max-width: 820px) {
    .stat-strip {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .spec-grid {
      grid-template-columns: 1fr;
    }

    .spec-row:nth-child(2) {
      border-top: 1px solid var(--border-subtle);
      padding-top: var(--space-2);
    }
  }

  @media (max-width: 480px) {
    .stat-strip {
      grid-template-columns: 1fr;
    }

    .spec-row {
      grid-template-columns: 1fr;
      gap: 0.15rem;
    }
  }
</style>
