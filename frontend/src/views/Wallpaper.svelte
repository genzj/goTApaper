<script lang="ts">
  import { ApiError, api, type LastApplied } from '../lib/api'
  import AuthImage from '../lib/AuthImage.svelte'
  import { bytes, unixTime } from '../lib/format'
  import { configVersion, downloading, lastDownload } from '../lib/events'

  let last = $state<LastApplied | null>(null)
  let error = $state('')
  // Bumped to refetch the image after a download: the path is the same but
  // the picture behind it is not.
  let imageVersion = $state(0)

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
</script>

<div class="row">
  <button onclick={() => refreshNow(false)} disabled={$downloading !== null}>
    {$downloading ? 'Downloading…' : 'Refresh now'}
  </button>
  <button
    onclick={() => refreshNow(true)}
    disabled={$downloading !== null}
    title="Download even if the channel would serve the same picture again"
  >
    Force refresh
  </button>
  <button onclick={load}>Reload</button>
</div>

{#if error}
  <p class="banner">{error}</p>
{:else if failed}
  <p class="banner">The last download failed: {failed}</p>
{/if}

<h2>Last applied wallpaper</h2>
<p class="muted">
  What goTApaper set most recently. If the wallpaper was changed by hand since,
  the desktop shows something else.
</p>

{#if last === null}
  <p class="muted">Loading…</p>
{:else if !last.entry}
  <p class="muted">
    No wallpaper has been applied yet. Use <strong>Refresh now</strong> to download one.
  </p>
{:else}
  <div class="preview">
    {#if last.exists}
      <AuthImage
        path="/wallpaper/last-applied/image"
        alt={last.entry.title ?? 'Last applied wallpaper'}
        version={imageVersion}
      />
    {:else}
      <p class="muted">
        The wallpaper file is no longer on disk, so there is nothing to preview.
      </p>
    {/if}

    <table>
      <tbody>
        <tr><th>Title</th><td>{last.entry.title || '—'}</td></tr>
        <tr><th>Caption</th><td>{last.entry.caption || '—'}</td></tr>
        <tr><th>Credit</th><td>{last.entry.credit || '—'}</td></tr>
        <tr><th>Channel</th><td>{last.entry.channelName} <span class="muted">({last.entry.channelType})</span></td></tr>
        <tr><th>Applied</th><td>{unixTime(last.entry.downloadTime)}</td></tr>
        <tr><th>Uploaded</th><td>{unixTime(last.entry.uploadTime)}</td></tr>
        <tr>
          <th>Dimensions</th>
          <td>{last.width ? `${last.width} × ${last.height}` : '—'}</td>
        </tr>
        <tr><th>Size</th><td>{bytes(last.size)}</td></tr>
        <tr><th>File</th><td class="mono">{last.path || '—'}</td></tr>
        <tr>
          <th>Source</th>
          <td class="mono break">{last.entry.url || '—'}</td>
        </tr>
      </tbody>
    </table>
  </div>
{/if}

<style>
  .preview :global(img) {
    margin-bottom: 1rem;
  }

  .break {
    overflow-wrap: anywhere;
  }

  th {
    width: 9rem;
  }
</style>
