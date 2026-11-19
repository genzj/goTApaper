<script lang="ts">
  import AuthImage from './AuthImage.svelte'
  import Icon from './Icon.svelte'

  export interface LightboxSpec {
    label: string
    value: string
    mono?: boolean
  }

  let {
    path,
    version = 0,
    title = 'Wallpaper preview',
    subtitle = '',
    caption = '',
    badges = [],
    specs = [],
    canReapply = false,
    reapplyDisabled = false,
    onReapply,
    onClose,
  }: {
    path: string
    version?: number
    title?: string
    subtitle?: string
    caption?: string
    badges?: string[]
    specs?: LightboxSpec[]
    canReapply?: boolean
    reapplyDisabled?: boolean
    onReapply?: () => void
    onClose: () => void
  } = $props()

  const ZOOM_STEPS = [1, 1.5, 2, 3, 4] as const
  let zoom = $state<number>(1)
  let showDetails = $state(true)
  let closeBtn = $state<HTMLButtonElement | null>(null)
  let stageEl = $state<HTMLDivElement | null>(null)
  let stageWidth = $state(0)
  let stageHeight = $state(0)
  let zoomPulse = $state(false)
  let zoomPulseTimer: number | undefined

  // Pointer drag-to-pan state when zoomed in (> 100%).
  let isDragging = $state(false)
  let dragMoved = false
  let dragStartX = 0
  let dragStartY = 0
  let startScrollLeft = 0
  let startScrollTop = 0

  $effect(() => {
    closeBtn?.focus()
    return () => {
      if (zoomPulseTimer) window.clearTimeout(zoomPulseTimer)
    }
  })

  $effect(() => {
    if (!stageEl) return
    const update = () => {
      if (!stageEl) return
      const w = stageEl.clientWidth
      const h = stageEl.clientHeight
      // Threshold guard (> 2px) prevents scrollbar appearance or sub-pixel
      // rounding from causing a ResizeObserver reflow loop.
      if (Math.abs(w - stageWidth) > 2) stageWidth = w
      if (Math.abs(h - stageHeight) > 2) stageHeight = h
    }
    update()
    const ro = new ResizeObserver(update)
    ro.observe(stageEl)
    return () => ro.disconnect()
  })

  const baseWidth = $derived(Math.max(240, stageWidth - 32))
  const baseHeight = $derived(Math.max(180, stageHeight - 32))

  function applyZoom(nextZoom: number) {
    if (nextZoom === zoom) return
    zoom = nextZoom
    zoomPulse = true
    if (zoomPulseTimer) window.clearTimeout(zoomPulseTimer)
    zoomPulseTimer = window.setTimeout(() => {
      zoomPulse = false
    }, 180)
  }

  function stepZoom(delta: number) {
    const idx = ZOOM_STEPS.findIndex((z) => z >= zoom)
    const cur = idx === -1 ? ZOOM_STEPS.length - 1 : idx
    const next = Math.max(0, Math.min(ZOOM_STEPS.length - 1, cur + delta))
    applyZoom(ZOOM_STEPS[next])
  }

  function toggleQuickZoom() {
    if (dragMoved) {
      dragMoved = false
      return
    }
    applyZoom(zoom === 1 ? 2 : 1)
  }

  function onStagePointerDown(e: PointerEvent) {
    if (zoom <= 1 || e.button !== 0 || !stageEl) return
    isDragging = true
    dragMoved = false
    dragStartX = e.clientX
    dragStartY = e.clientY
    startScrollLeft = stageEl.scrollLeft
    startScrollTop = stageEl.scrollTop
  }

  function onStagePointerMove(e: PointerEvent) {
    if (!isDragging || !stageEl) return
    const dx = e.clientX - dragStartX
    const dy = e.clientY - dragStartY
    if (Math.abs(dx) > 4 || Math.abs(dy) > 4) {
      dragMoved = true
    }
    stageEl.scrollLeft = startScrollLeft - dx
    stageEl.scrollTop = startScrollTop - dy
  }

  function onStagePointerUp() {
    isDragging = false
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault()
      onClose()
    } else if (e.key === '+' || e.key === '=') {
      e.preventDefault()
      stepZoom(1)
    } else if (e.key === '-') {
      e.preventDefault()
      stepZoom(-1)
    } else if (e.key === '0') {
      e.preventDefault()
      applyZoom(1)
    } else if (e.key === 'i' || e.key === 'I') {
      e.preventDefault()
      showDetails = !showDetails
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

<!-- svelte-ignore a11y_click_events_have_key_events -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div
  class="backdrop"
  role="dialog"
  aria-modal="true"
  aria-label={title}
  tabindex="-1"
  onclick={(e) => {
    if (e.target === e.currentTarget) onClose()
  }}
>
  <div class="lightbox">
    <header class="lb-header">
      <div class="lb-title-group">
        <div class="lb-badges">
          {#each badges as b (b)}
            <span class="badge accent">{b}</span>
          {/each}
        </div>
        <div class="lb-headings">
          <h2>{title}</h2>
          {#if subtitle}
            <span class="muted">{subtitle}</span>
          {/if}
        </div>
      </div>

      <div class="lb-controls">
        <div class="zoom-group" role="group" aria-label="Zoom controls">
          <button
            type="button"
            onclick={() => stepZoom(-1)}
            disabled={zoom <= ZOOM_STEPS[0]}
            title="Zoom out (-)"
            aria-label="Zoom out"
          >
            <Icon name="zoom-out" size={15} />
          </button>
          <button
            type="button"
            class="zoom-readout"
            onclick={() => applyZoom(1)}
            title="Reset zoom to fit (0)"
          >
            {Math.round(zoom * 100)}%
          </button>
          <button
            type="button"
            onclick={() => stepZoom(1)}
            disabled={zoom >= ZOOM_STEPS[ZOOM_STEPS.length - 1]}
            title="Zoom in (+)"
            aria-label="Zoom in"
          >
            <Icon name="zoom-in" size={15} />
          </button>
        </div>

        {#if specs.length > 0 || caption}
          <button
            type="button"
            class:active-toggle={showDetails}
            onclick={() => (showDetails = !showDetails)}
            title="Toggle metadata drawer (I)"
          >
            <Icon name="sliders" size={15} />
            <span>Details</span>
          </button>
        {/if}

        {#if canReapply && onReapply}
          <button
            type="button"
            class="btn-primary"
            onclick={onReapply}
            disabled={reapplyDisabled}
          >
            <Icon name="refresh" size={15} />
            <span>Re-apply</span>
          </button>
        {/if}

        <button
          type="button"
          class="close-btn"
          bind:this={closeBtn}
          onclick={onClose}
          title="Close inspector (Esc)"
          aria-label="Close inspector"
        >
          <Icon name="close" size={17} />
        </button>
      </div>
    </header>

    <div class="lb-body">
      <div
        class="stage"
        class:zoomed={zoom > 1}
        class:dragging={isDragging}
        role="region"
        aria-label="Zoomable photo stage"
        bind:this={stageEl}
        onpointerdown={onStagePointerDown}
        onpointermove={onStagePointerMove}
        onpointerup={onStagePointerUp}
        onpointercancel={onStagePointerUp}
        onpointerleave={onStagePointerUp}
      >
        <button
          type="button"
          class="canvas-trigger"
          class:zoom-pulse={zoomPulse}
          onclick={toggleQuickZoom}
          title={zoom === 1 ? 'Click to zoom in (200%)' : 'Drag to pan · Click to fit (100%)'}
          style={`width: ${Math.round(baseWidth * zoom)}px; --zoom-max-h: ${Math.round(baseHeight * zoom)}px;`}
        >
          <AuthImage {path} alt={title} {version} />
        </button>
      </div>

      {#if showDetails && (specs.length > 0 || caption)}
        <aside class="lb-drawer">
          {#if caption}
            <div class="drawer-section">
              <h3>Caption</h3>
              <p class="drawer-caption">{caption}</p>
            </div>
          {/if}

          {#if specs.length > 0}
            <div class="drawer-section">
              <h3>Specifications</h3>
              <dl class="drawer-specs">
                {#each specs as item (item.label)}
                  <div class="drawer-row">
                    <dt>{item.label}</dt>
                    <dd class:mono={item.mono}>{item.value}</dd>
                  </div>
                {/each}
              </dl>
            </div>
          {/if}
        </aside>
      {/if}
    </div>
  </div>
</div>

<style>
  @keyframes backdrop-fade {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  @keyframes modal-enter {
    from {
      opacity: 0;
      transform: translateY(8px) scale(0.985);
    }
    to {
      opacity: 1;
      transform: translateY(0) scale(1);
    }
  }

  @keyframes drawer-enter {
    from {
      opacity: 0;
      transform: translateX(8px);
    }
    to {
      opacity: 1;
      transform: translateX(0);
    }
  }

  @keyframes zoom-snap {
    from {
      opacity: 0.88;
      transform: scale(0.985);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 100;
    /* Solid/translucent studio darkroom scrim without backdrop-filter blur:
       avoids full-viewport GPU background re-blurring on every frame. */
    background: rgba(9, 9, 11, 0.9);
    contain: layout style paint;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    animation: backdrop-fade var(--duration-fast) var(--ease-out);
  }

  .lightbox {
    width: 100%;
    max-width: 96vw;
    height: 92dvh;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
    display: flex;
    flex-direction: column;
    overflow: hidden;
    contain: layout paint;
    animation: modal-enter var(--duration-normal) var(--ease-out);
  }

  .lb-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4);
    background: var(--surface);
    border-bottom: 1px solid var(--border);
    flex-shrink: 0;
  }

  .lb-title-group {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-3);
    min-width: 0;
  }

  .lb-badges {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .lb-headings {
    display: flex;
    align-items: baseline;
    flex-wrap: wrap;
    gap: var(--space-2);
    min-width: 0;
  }

  .lb-headings h2 {
    margin: 0;
    font-size: var(--text-md);
    font-weight: 700;
  }

  .lb-headings span {
    font-size: var(--text-xs);
  }

  .lb-controls {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .zoom-group {
    display: inline-flex;
    align-items: center;
    background: var(--surface-subtle);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    padding: 0.1rem;
    gap: 0.1rem;
  }

  .zoom-group button {
    border: none;
    background: transparent;
    box-shadow: none;
    min-height: 1.85rem;
    padding: 0 var(--space-2);
  }

  .zoom-group button:hover:not(:disabled) {
    background: var(--surface-hover);
  }

  .zoom-readout {
    min-width: 3.4rem;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: 600;
  }

  .active-toggle {
    background: var(--accent-surface);
    border-color: var(--accent-border);
    color: var(--accent);
  }

  .close-btn {
    width: 2.125rem;
    padding: 0;
  }

  .lb-body {
    flex: 1;
    min-height: 0;
    display: flex;
    background: var(--bg);
  }

  .stage {
    flex: 1;
    min-width: 0;
    min-height: 0;
    overflow: auto;
    contain: strict;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    background: var(--bg);
    user-select: none;
  }

  .stage.zoomed {
    align-items: flex-start;
    justify-content: flex-start;
    cursor: grab;
  }

  .stage.zoomed.dragging {
    cursor: grabbing;
  }

  .canvas-trigger {
    flex: 0 0 auto;
    display: block;
    padding: 0;
    border: none;
    background: transparent;
    box-shadow: none;
    cursor: zoom-in;
    border-radius: var(--radius-md);
  }

  .canvas-trigger.zoom-pulse {
    animation: zoom-snap var(--duration-fast) var(--ease-out);
  }

  .stage.zoomed .canvas-trigger {
    cursor: inherit;
  }

  .canvas-trigger:hover:not(:disabled) {
    background: transparent;
    border-color: transparent;
  }

  .canvas-trigger :global(img) {
    width: 100%;
    height: auto;
    max-height: var(--zoom-max-h, 76dvh);
    object-fit: contain;
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-sm);
    background: var(--surface);
    pointer-events: none;
  }

  .stage.zoomed .canvas-trigger :global(img) {
    box-shadow: none;
  }

  .lb-drawer {
    width: 20rem;
    flex-shrink: 0;
    background: var(--surface);
    border-left: 1px solid var(--border);
    padding: var(--space-4);
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
    animation: drawer-enter var(--duration-fast) var(--ease-out);
  }

  .drawer-section h3 {
    margin: 0 0 var(--space-2);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--muted);
  }

  .drawer-caption {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--fg-secondary);
    line-height: 1.55;
  }

  .drawer-specs {
    margin: 0;
    display: flex;
    flex-direction: column;
  }

  .drawer-row {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--border-subtle);
  }

  .drawer-row:last-child {
    border-bottom: none;
  }

  .drawer-row dt {
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
  }

  .drawer-row dd {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--fg);
    overflow-wrap: anywhere;
    word-break: break-word;
  }

  @media (max-width: 820px) {
    .backdrop {
      padding: var(--space-2);
    }

    .lightbox {
      max-width: 100vw;
      height: 95dvh;
    }

    .lb-body {
      flex-direction: column;
    }

    .lb-drawer {
      width: 100%;
      max-height: 38%;
      border-left: none;
      border-top: 1px solid var(--border);
    }
  }
</style>
