<script lang="ts">
  // An image from the API.
  //
  // A plain <img src> cannot send the Authorization header, so the picture is
  // fetched with the token and shown from a blob: URL, which is revoked
  // when the source changes or the component goes, so pictures do not pile up
  // in memory while scrolling the history.

  import { authorizedFetch } from './api'

  let {
    path,
    alt = '',
    /** Changes to force a refetch of the same path, e.g. after a refresh. */
    version = 0,
  }: { path: string; alt?: string; version?: number } = $props()

  let src = $state('')
  let failed = $state(false)

  $effect(() => {
    void version
    let objectURL = ''
    let cancelled = false
    failed = false

    authorizedFetch(path)
      .then((resp) => (resp.ok ? resp.blob() : Promise.reject(resp.status)))
      .then((blob) => {
        if (cancelled) return
        objectURL = URL.createObjectURL(blob)
        src = objectURL
      })
      .catch(() => {
        if (!cancelled) failed = true
      })

    return () => {
      cancelled = true
      if (objectURL) URL.revokeObjectURL(objectURL)
    }
  })
</script>

{#if failed}
  <div class="missing muted">image unavailable</div>
{:else if src}
  <img {src} {alt} />
{:else}
  <div class="missing muted">loading…</div>
{/if}

<style>
  img,
  .missing {
    width: 100%;
    display: block;
    border: 1px solid var(--border);
    border-radius: 6px;
  }

  .missing {
    aspect-ratio: 16 / 9;
    display: grid;
    place-items: center;
    font-size: 0.8rem;
  }
</style>
