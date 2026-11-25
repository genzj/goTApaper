<script lang="ts">
  // The preset gallery: each built-in preset rendered small on the current
  // picture, by the same renderer, so a card shows what picking it gives.

  import { api, type WatermarkPreset } from '../api'

  let {
    presets,
    onPick,
    onClose,
  }: {
    presets: WatermarkPreset[]
    onPick: (preset: WatermarkPreset) => void
    onClose: () => void
  } = $props()

  let samples = $state<Record<string, string>>({})

  $effect(() => {
    const urls: string[] = []
    let cancelled = false
    // One at a time: each is a render on the daemon, and the first card
    // appearing quickly beats all of them appearing late.
    ;(async () => {
      for (const p of presets) {
        try {
          const blob = await api.previewSample(p.layers, 480)
          if (cancelled) return
          const url = URL.createObjectURL(blob)
          urls.push(url)
          samples = { ...samples, [p.id]: url }
        } catch {
          // The card shows its description alone.
        }
      }
    })()
    return () => {
      cancelled = true
      urls.forEach((u) => URL.revokeObjectURL(u))
    }
  })

  function keydown(e: KeyboardEvent) {
    if (e.key === 'Escape') onClose()
  }
</script>

<svelte:window onkeydown={keydown} />

<div class="scrim" role="presentation" onclick={onClose}></div>
<div class="dialog" role="dialog" aria-modal="true" aria-labelledby="preset-title">
  <div class="head">
    <h2 id="preset-title">Add a preset</h2>
    <button type="button" onclick={onClose} aria-label="Close">✕</button>
  </div>
  <p class="muted intro">
    Shown on the current picture. Adding a preset puts its layers on top of yours;
    your existing layers are kept. Undo takes it off again.
  </p>
  <div class="grid">
    {#each presets as p (p.id)}
      <article class="card-preset">
        <div class="thumb">
          {#if samples[p.id]}
            <img src={samples[p.id]} alt={`${p.name} preview`} decoding="async" />
          {:else}
            <span class="muted">Rendering…</span>
          {/if}
        </div>
        <div class="body">
          <h3>{p.name}</h3>
          <p class="muted">{p.description}</p>
          <div class="actions">
            <button type="button" class="btn-primary" onclick={() => onPick(p)}>
              Add {p.layers.length > 1 ? `${p.layers.length} layers` : 'layer'}
            </button>
          </div>
        </div>
      </article>
    {/each}
  </div>
</div>

<style>
  /* A flat scrim, not a blur: blurring the 4K preview behind it would make
     every frame inside the dialog re-sample the whole picture. */
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 50;
    background: rgba(9, 9, 11, 0.9);
    contain: layout style paint;
  }

  .dialog {
    position: fixed;
    z-index: 51;
    inset: 5vh 5vw;
    max-width: 68rem;
    margin: 0 auto;
    overflow: auto;
    padding: var(--space-4) var(--space-5);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
  }

  .head {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  h2 {
    margin: 0;
  }

  .intro {
    font-size: var(--text-sm);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    gap: var(--space-3);
  }

  .card-preset {
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    overflow: hidden;
    display: flex;
    flex-direction: column;
    transition: border-color var(--duration-fast) var(--ease-out);
  }

  .card-preset:hover {
    border-color: var(--accent-border);
    box-shadow: 0 0 0 3px var(--accent-surface);
  }

  .thumb {
    aspect-ratio: 16 / 9;
    display: grid;
    place-items: center;
    background: var(--surface-subtle);
  }

  .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .body {
    padding: var(--space-2) var(--space-3) var(--space-3);
  }

  h3 {
    margin: 0 0 var(--space-1);
    font-size: var(--text-md);
  }

  .body p {
    margin: 0 0 var(--space-2);
    font-size: var(--text-xs);
  }

  .actions {
    display: flex;
    gap: var(--space-2);
  }
</style>
