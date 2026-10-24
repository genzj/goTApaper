<script lang="ts">
  import { api, imageURL, type CurrentWallpaper } from '../lib/api'
  import { bytes, unixTime } from '../lib/format'
  import { configVersion, lastRefresh, refreshing } from '../lib/events'

  let current = $state<CurrentWallpaper | null>(null)
  let error = $state('')
  // Appended to the image URL to defeat the browser's own cache after a
  // refresh: the path is the same but the picture behind it is not.
  let imageKey = $state(Date.now())

  async function load() {
    try {
      current = await api.currentWallpaper()
      imageKey = Date.now()
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  async function refreshNow() {
    error = ''
    try {
      await api.refresh()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
    await load()
  }

  // Reload whenever configuration changes or a refresh finishes anywhere,
  // including one started from the tray or the CLI.
  $effect(() => {
    void $configVersion
    void $lastRefresh
    load()
  })
</script>

<div class="row">
  <button onclick={refreshNow} disabled={$refreshing}>
    {$refreshing ? 'Refreshing…' : 'Refresh now'}
  </button>
  <button onclick={load}>Reload</button>
</div>

{#if error}
  <p class="banner">{error}</p>
{/if}

{#if current === null}
  <p class="muted">Loading…</p>
{:else if !current.entry}
  <p class="muted">
    No wallpaper has been applied yet. Use <strong>Refresh now</strong> to download one.
  </p>
{:else}
  <div class="preview">
    {#if current.exists}
      <img src={`${imageURL('/wallpaper/current/image')}?v=${imageKey}`} alt={current.entry.title ?? 'Current wallpaper'} />
    {:else}
      <p class="muted">
        The wallpaper file is no longer on disk, so there is nothing to preview.
      </p>
    {/if}

    <table>
      <tbody>
        <tr><th>Title</th><td>{current.entry.title || '—'}</td></tr>
        <tr><th>Caption</th><td>{current.entry.caption || '—'}</td></tr>
        <tr><th>Credit</th><td>{current.entry.credit || '—'}</td></tr>
        <tr><th>Channel</th><td>{current.entry.channelName} <span class="muted">({current.entry.channelType})</span></td></tr>
        <tr><th>Applied</th><td>{unixTime(current.entry.downloadTime)}</td></tr>
        <tr><th>Uploaded</th><td>{unixTime(current.entry.uploadTime)}</td></tr>
        <tr>
          <th>Dimensions</th>
          <td>{current.width ? `${current.width} × ${current.height}` : '—'}</td>
        </tr>
        <tr><th>Size</th><td>{bytes(current.size)}</td></tr>
        <tr><th>File</th><td class="mono">{current.path || '—'}</td></tr>
        <tr>
          <th>Source</th>
          <td class="mono break">{current.entry.url || '—'}</td>
        </tr>
      </tbody>
    </table>
  </div>
{/if}

<style>
  .preview img {
    max-width: 100%;
    border: 1px solid var(--border);
    border-radius: 6px;
    display: block;
    margin-bottom: 1rem;
  }

  .break {
    overflow-wrap: anywhere;
  }

  th {
    width: 9rem;
  }
</style>
