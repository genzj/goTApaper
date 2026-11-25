<script lang="ts">
  // The preview stage: the server's render of the draft, with a handle over
  // each layer to select and drag it.
  //
  // What is shown is always the server's own render -- the same file the
  // desktop preview sets -- so it is exact rather than an approximation drawn
  // in the browser. While a layer is dragged only its handle moves (a CSS
  // transform, which the compositor does alone), carrying a patch of the
  // render so the text appears to move with the pointer; the layer is
  // rendered again once it is dropped.

  import {
    api,
    authorizedFetch,
    type FractionRect,
    type LayerResult,
    type WatermarkLayer,
  } from '../api'
  import { ASPECT_GUIDES, guideRect, layerLabel, placeFromBox } from './model'

  let {
    revision,
    pictureWidth,
    pictureHeight,
    layers,
    results,
    selected,
    busy = false,
    guides = [],
    onSelect,
    onDrop,
    onNudge,
  }: {
    revision: number
    pictureWidth: number
    pictureHeight: number
    layers: WatermarkLayer[]
    results: LayerResult[]
    selected: number
    busy?: boolean
    guides?: string[]
    onSelect: (index: number) => void
    onDrop: (index: number, box: FractionRect) => void
    onNudge: (index: number, dx: number, dy: number) => void
  } = $props()

  let stage = $state<HTMLDivElement>()
  let stageW = $state(0)
  let stageH = $state(0)

  // Measured, with a threshold: a scrollbar appearing nearby shifts the width
  // by a few pixels, and reacting to that would refetch for nothing.
  $effect(() => {
    if (!stage) return
    const ro = new ResizeObserver(([entry]) => {
      const { width, height } = entry.contentRect
      if (Math.abs(width - stageW) > 2) stageW = width
      if (Math.abs(height - stageH) > 2) stageH = height
    })
    ro.observe(stage)
    return () => ro.disconnect()
  })

  // Asked for at the stage's size in device pixels, so it is crisp on a
  // HiDPI screen, in 128-pixel steps so resizing the window does not refetch
  // on every pixel.
  const requestWidth = $derived(
    stageW > 0
      ? Math.min(2560, Math.ceil((stageW * (window.devicePixelRatio || 1)) / 128) * 128)
      : 0,
  )

  // Double-buffered: the previous render stays up until the next one has
  // decoded, so editing never flashes a placeholder.
  let shown = $state('')
  let loadError = $state('')
  $effect(() => {
    if (!revision || !requestWidth) return
    const path = api.previewImagePath(requestWidth, revision)
    let cancelled = false
    let url = ''
    authorizedFetch(path)
      .then((resp) => (resp.ok ? resp.blob() : Promise.reject(new Error(`HTTP ${resp.status}`))))
      .then(async (blob) => {
        if (cancelled) return
        url = URL.createObjectURL(blob)
        const img = new Image()
        img.decoding = 'async'
        img.src = url
        await img.decode().catch(() => {})
        if (cancelled) return
        const previous = shown
        shown = url
        loadError = ''
        if (previous) URL.revokeObjectURL(previous)
      })
      .catch((e) => {
        if (!cancelled) loadError = e instanceof Error ? e.message : String(e)
      })
    return () => {
      cancelled = true
      // Revoked here only if it never made it on screen.
      if (url && url !== shown) URL.revokeObjectURL(url)
    }
  })
  $effect(() => () => {
    if (shown) URL.revokeObjectURL(shown)
  })

  // ---- dragging ----------------------------------------------------------

  type Drag = {
    index: number
    pointer: number
    startX: number
    startY: number
    box: FractionRect
    dx: number
    dy: number
    moved: boolean
  }
  let drag = $state<Drag | null>(null)
  let snapX = $state<{ line: number; label: string } | null>(null)
  let snapY = $state<{ line: number; label: string } | null>(null)
  /** A dropped layer keeps its new place until the render that moves it. */
  let pending = $state<{ index: number; dx: number; dy: number; revision: number } | null>(null)

  $effect(() => {
    if (pending && revision !== pending.revision) pending = null
  })

  const SNAP_PX = 6

  type Snap = { line: number; label: string }

  // What each guide is called, per axis, so the label says where the text
  // snapped rather than leaving the user to guess from a bare line.
  const snapNames = {
    x: { 0: 'Left edge', 1: 'Right edge', centre: 'Horizontal centre', third: ['Left third', 'Right third'] },
    y: { 0: 'Top edge', 1: 'Bottom edge', centre: 'Vertical centre', third: ['Upper third', 'Lower third'] },
  }

  /**
   * snapped pulls a drag onto the nearest guide within a few pixels: the
   * box's centre onto the picture's centre or thirds, or its edge onto the
   * picture's edge. It returns the adjusted delta and the guide, if any.
   */
  function snapped(
    axis: 'x' | 'y',
    start: number,
    size: number,
    delta: number,
    extent: number,
  ): [number, Snap | null] {
    const names = snapNames[axis]
    const threshold = SNAP_PX / extent
    const candidates: [number, Snap][] = [
      [0 - start, { line: 0, label: names[0] }],
      [1 - size - start, { line: 1, label: names[1] }],
      [0.5 - size / 2 - start, { line: 0.5, label: names.centre }],
      [1 / 3 - size / 2 - start, { line: 1 / 3, label: names.third[0] }],
      [2 / 3 - size / 2 - start, { line: 2 / 3, label: names.third[1] }],
    ]
    let best: [number, Snap | null] = [delta, null]
    let bestDist = threshold
    for (const [d, snap] of candidates) {
      const dist = Math.abs(d - delta)
      if (dist < bestDist) {
        best = [d, snap]
        bestDist = dist
      }
    }
    return best
  }

  function pointerDown(e: PointerEvent, index: number) {
    if (e.button !== 0) return
    const r = results[index]
    if (!r) return
    onSelect(index)
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    drag = {
      index,
      pointer: e.pointerId,
      startX: e.clientX,
      startY: e.clientY,
      box: r.textBounds,
      dx: 0,
      dy: 0,
      moved: false,
    }
    e.preventDefault()
  }

  function pointerMove(e: PointerEvent) {
    if (!drag || e.pointerId !== drag.pointer || stageW === 0) return
    const rawDx = (e.clientX - drag.startX) / stageW
    const rawDy = (e.clientY - drag.startY) / stageH
    if (!drag.moved && Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY) < 3) return
    drag.moved = true
    if (e.altKey) {
      // Alt drags freely, without snapping.
      drag.dx = rawDx
      drag.dy = rawDy
      snapX = snapY = null
      return
    }
    ;[drag.dx, snapX] = snapped('x', drag.box.x, drag.box.w, rawDx, stageW)
    ;[drag.dy, snapY] = snapped('y', drag.box.y, drag.box.h, rawDy, stageH)
  }

  function pointerUp(e: PointerEvent) {
    if (!drag || e.pointerId !== drag.pointer) return
    const d = drag
    drag = null
    snapX = snapY = null
    if (!d.moved) return
    pending = { index: d.index, dx: d.dx, dy: d.dy, revision }
    onDrop(d.index, { x: d.box.x + d.dx, y: d.box.y + d.dy, w: d.box.w, h: d.box.h })
  }

  function keyDown(e: KeyboardEvent, index: number) {
    const step = (e.shiftKey ? 10 : 1) / Math.max(stageW, 1)
    const stepY = (e.shiftKey ? 10 : 1) / Math.max(stageH, 1)
    const moves: Record<string, [number, number]> = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -stepY],
      ArrowDown: [0, stepY],
    }
    const m = moves[e.key]
    if (!m) return
    e.preventDefault()
    onNudge(index, m[0], m[1])
  }

  function offsetOf(index: number): [number, number] {
    if (drag && drag.index === index) return [drag.dx, drag.dy]
    if (pending && pending.index === index) return [pending.dx, pending.dy]
    return [0, 0]
  }

  const pictureRatio = $derived(pictureWidth > 0 && pictureHeight > 0 ? pictureWidth / pictureHeight : 16 / 9)
  const guideRects = $derived(
    ASPECT_GUIDES.filter((g) => guides.includes(g.label)).map((g) => ({
      label: g.label,
      rect: guideRect(pictureRatio, g.ratio),
    })),
  )

  const pct = (v: number) => `${(v * 100).toFixed(4)}%`
</script>

<div class="stage-wrap">
  <div
    class="stage"
    class:busy
    bind:this={stage}
    style:aspect-ratio={`${pictureWidth || 16} / ${pictureHeight || 9}`}
    style:--ratio={pictureRatio}
    role="application"
    aria-label="Watermark preview. Select a layer and drag it, or use the arrow keys to move it."
    onpointerdown={(e) => {
      if (e.target === stage || (e.target as HTMLElement).tagName === 'IMG') onSelect(-1)
    }}
  >
    {#if shown}
      <img src={shown} alt="Watermark preview" decoding="async" draggable="false" />
    {:else}
      <div class="placeholder muted" role="status">
        <span class="spinner" aria-hidden="true"></span>
        {loadError ? `Preview unavailable: ${loadError}` : 'Rendering preview…'}
      </div>
    {/if}

    {#each guideRects as g (g.label)}
      <div
        class="aspect-guide"
        style:left={pct(g.rect.x)}
        style:top={pct(g.rect.y)}
        style:width={pct(g.rect.w)}
        style:height={pct(g.rect.h)}
      >
        <span>{g.label}</span>
      </div>
    {/each}

    {#if snapX}
      <div class="snap-line v" style:left={pct(snapX.line)}></div>
      <span
        class="snap-label v"
        class:flip={snapX.line > 0.75}
        style:left={pct(snapX.line)}
      >{snapX.label}</span>
    {/if}
    {#if snapY}
      <div class="snap-line h" style:top={pct(snapY.line)}></div>
      <span
        class="snap-label h"
        class:flip={snapY.line > 0.75}
        style:top={pct(snapY.line)}
      >{snapY.label}</span>
    {/if}
    {#if drag?.moved}
      {@const box = { x: drag.box.x + drag.dx, y: drag.box.y + drag.dy, w: drag.box.w, h: drag.box.h }}
      {@const place = placeFromBox(box)}
      <span class="drop-readout" role="status" aria-live="polite">
        {[snapX?.label, snapY?.label].filter(Boolean).join(' · ') || 'Free position'}
        — anchor {place.anchor}, x {(place.x * 100).toFixed(1)}%, y {(place.y * 100).toFixed(1)}%
        {#if !snapX && !snapY}<span class="hint">(Alt: no snapping)</span>{/if}
      </span>
    {/if}

    {#each results as r, i (i)}
      {@const layer = layers[i]}
      {#if layer && r.enabled && r.text && !r.error}
        {@const [dx, dy] = offsetOf(i)}
        {@const dragging = drag?.index === i && drag.moved}
        <div
          class="handle"
          class:selected={selected === i}
          class:dragging
          role="button"
          tabindex="0"
          aria-label={`${layerLabel(layer, i)}: ${r.text}`}
          aria-pressed={selected === i}
          style:left={pct(r.textBounds.x)}
          style:top={pct(r.textBounds.y)}
          style:width={pct(r.textBounds.w)}
          style:height={pct(r.textBounds.h)}
          style:transform={`translate(${dx * stageW}px, ${dy * stageH}px)`}
          style:background-image={dragging && shown ? `url(${shown})` : 'none'}
          style:background-size={`${stageW}px ${stageH}px`}
          style:background-position={`${-r.textBounds.x * stageW}px ${-r.textBounds.y * stageH}px`}
          onpointerdown={(e) => pointerDown(e, i)}
          onpointermove={pointerMove}
          onpointerup={pointerUp}
          onpointercancel={pointerUp}
          onkeydown={(e) => keyDown(e, i)}
          onfocus={() => onSelect(i)}
        >
          {#if selected === i}
            <span class="tag">{layerLabel(layer, i)}</span>
          {/if}
        </div>
      {/if}
    {/each}
  </div>
</div>

<style>
  .stage-wrap {
    width: 100%;
    display: flex;
    justify-content: center;
  }

  .stage {
    position: relative;
    width: 100%;
    max-height: 72vh;
    /* Width follows the height cap, so the stage keeps the picture's ratio. */
    max-width: calc(72vh * var(--ratio, 1.78));
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    overflow: hidden;
    contain: strict;
    touch-action: none;
    user-select: none;
  }

  .stage img {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    display: block;
    pointer-events: auto;
  }

  .stage.busy img {
    opacity: 0.92;
    transition: opacity var(--duration-fast) var(--ease-out);
  }

  .placeholder {
    position: absolute;
    inset: 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
  }

  .spinner {
    width: 1.5rem;
    height: 1.5rem;
    border-radius: var(--radius-pill);
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    animation: spin 750ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .handle {
    position: absolute;
    box-sizing: content-box;
    margin: -4px 0 0 -4px;
    padding: 4px;
    border: 1px dashed transparent;
    border-radius: 4px;
    cursor: grab;
    background-repeat: no-repeat;
    background-origin: content-box;
    will-change: transform;
    contain: layout style;
  }

  .handle:hover,
  .handle:focus-visible {
    border-color: rgba(255, 255, 255, 0.75);
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.45);
    outline: none;
  }

  .handle.selected {
    border: 1px solid var(--accent);
    box-shadow:
      0 0 0 1px rgba(255, 255, 255, 0.6),
      0 0 0 3px var(--accent-surface);
  }

  .handle.dragging {
    cursor: grabbing;
    box-shadow:
      0 0 0 1px var(--accent),
      0 6px 18px rgba(0, 0, 0, 0.35);
  }

  .tag {
    position: absolute;
    left: -1px;
    bottom: 100%;
    margin-bottom: 4px;
    padding: 0.05rem 0.4rem;
    font-size: var(--text-xs);
    font-weight: 600;
    white-space: nowrap;
    color: var(--accent-fg);
    background: var(--accent);
    border-radius: 4px;
    pointer-events: none;
  }

  .snap-line {
    position: absolute;
    background: #ff3dbb;
    pointer-events: none;
  }

  .snap-line.v {
    top: 0;
    bottom: 0;
    width: 1px;
  }

  .snap-line.h {
    left: 0;
    right: 0;
    height: 1px;
  }

  .snap-label {
    position: absolute;
    z-index: 2;
    padding: 0.05rem 0.4rem;
    font-size: var(--text-xs);
    font-weight: 600;
    white-space: nowrap;
    color: #fff;
    background: #d6249f;
    border-radius: 4px;
    pointer-events: none;
  }

  /* Beside its line: a vertical guide is labelled at the top, a horizontal
     one at the left, moving to the other side near the far edge. */
  .snap-label.v {
    top: 6px;
    margin-left: 6px;
  }

  .snap-label.v.flip {
    transform: translateX(calc(-100% - 12px));
  }

  .snap-label.h {
    left: 6px;
    margin-top: 4px;
  }

  .snap-label.h.flip {
    transform: translateY(calc(-100% - 8px));
  }

  .drop-readout {
    position: absolute;
    z-index: 2;
    left: 50%;
    bottom: 8px;
    transform: translateX(-50%);
    padding: 0.2rem 0.6rem;
    font-size: var(--text-xs);
    font-weight: 500;
    white-space: nowrap;
    color: #fff;
    background: rgba(9, 9, 11, 0.85);
    border-radius: var(--radius-pill);
    pointer-events: none;
  }

  .drop-readout .hint {
    opacity: 0.7;
  }

  .aspect-guide {
    position: absolute;
    border: 1px dashed rgba(255, 214, 10, 0.9);
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35);
    pointer-events: none;
  }

  .aspect-guide span {
    position: absolute;
    top: 2px;
    left: 4px;
    font-size: var(--text-xs);
    font-weight: 600;
    color: #ffd60a;
    text-shadow: 0 1px 2px rgba(0, 0, 0, 0.8);
  }
</style>
