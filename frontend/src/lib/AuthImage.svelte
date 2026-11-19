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
  let loaded = $state(false)

  $effect(() => {
    void version
    let objectURL = ''
    let cancelled = false
    src = ''
    failed = false
    loaded = false

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

  function handleLoad(e: Event) {
    const img = e.currentTarget as HTMLImageElement
    if (typeof img.decode === 'function') {
      img.decode().catch(() => {}).finally(() => {
        if (img.src === src) loaded = true
      })
    } else {
      loaded = true
    }
  }
</script>

{#if failed}
  <div class="missing muted" role="status">image unavailable</div>
{:else}
  <div class="frame" class:loading={!loaded}>
    {#if !loaded}
      <div class="missing placeholder" role="status" aria-live="polite">
        <span class="spinner" aria-hidden="true"></span>
        <span class="label">{src ? 'Decoding photo…' : 'Loading photo…'}</span>
      </div>
    {/if}

    {#if src}
      <img
        {src}
        {alt}
        decoding="async"
        class:loaded
        onload={handleLoad}
        onerror={() => (failed = true)}
      />
    {/if}
  </div>
{/if}

<style>
  @keyframes auth-spin {
    to {
      transform: rotate(360deg);
    }
  }

  @keyframes auth-pulse {
    0%,
    100% {
      opacity: 0.65;
    }
    50% {
      opacity: 1;
    }
  }

  .frame {
    position: relative;
    width: 100%;
    display: block;
  }

  img,
  .missing {
    width: 100%;
    display: block;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-subtle);
  }

  img {
    box-shadow: var(--shadow-sm);
    opacity: 0;
    transition: opacity var(--duration-normal) var(--ease-out);
  }

  /* Keep the decoding image out of document flow until ready so the 16:9
     placeholder occupies stable space without collapsing or jumping. */
  .frame.loading img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: contain;
    pointer-events: none;
  }

  img.loaded {
    opacity: 1;
  }

  .missing {
    aspect-ratio: 16 / 9;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    font-size: var(--text-xs);
    font-weight: 500;
    letter-spacing: 0.02em;
    color: var(--muted);
  }

  .placeholder {
    position: relative;
    overflow: hidden;
  }

  .spinner {
    width: 1.5rem;
    height: 1.5rem;
    border-radius: var(--radius-pill);
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    animation: auth-spin 750ms linear infinite;
  }

  .label {
    animation: auth-pulse 1.6s ease-in-out infinite;
  }
</style>
