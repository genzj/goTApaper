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
  //
  // The picture can be zoomed to examine detail. Zoomed in, the stage scrolls
  // and dragging pans the picture instead of moving a layer: layers are locked
  // until the zoom is back to fit, so a pan can never move one by accident.
  // The server is asked for the picture at the zoomed size, so the detail is
  // real rather than an enlarged thumbnail.

  import { tick, untrack } from 'svelte'
  import Icon from '../Icon.svelte'
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
  let canvas = $state<HTMLDivElement>()
  // The picture's on-screen size (stageW x stageH) -- what layer geometry is
  // measured against -- and the viewport's, which is the picture at "fit".
  let stageW = $state(0)
  let stageH = $state(0)
  let viewW = $state(0)

  // Measured, with a threshold: a scrollbar appearing nearby shifts the width
  // by a few pixels, and reacting to that would refetch for nothing.
  $effect(() => {
    if (!canvas || !stage) return
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const { width, height } = entry.contentRect
        if (entry.target === canvas) {
          if (Math.abs(width - stageW) > 2) stageW = width
          if (Math.abs(height - stageH) > 2) stageH = height
        } else if (Math.abs(stage!.clientWidth - viewW) > 2) {
          viewW = stage!.clientWidth
        }
      }
    })
    ro.observe(canvas)
    ro.observe(stage)
    return () => ro.disconnect()
  })

  // ---- zoom --------------------------------------------------------------

  const dpr = () => window.devicePixelRatio || 1
  /** 1 is "fit": the whole picture in the viewport. */
  let zoom = $state(1)
  const zoomed = $derived(zoom > 1.001)

  // The zoom at which one picture pixel is one device pixel.
  const actualZoom = $derived(
    viewW > 0 && pictureWidth > 0 ? pictureWidth / dpr() / viewW : 1,
  )
  const maxZoom = $derived(Math.min(24, Math.max(8, actualZoom * 2)))
  /** The picture's on-screen size as a percentage of its real pixels. */
  const percent = $derived(
    pictureWidth > 0 && stageW > 0 ? Math.round(((stageW * dpr()) / pictureWidth) * 100) : 100,
  )

  /**
   * setZoom zooms about a point in the viewport (its centre by default), so
   * what is under the cursor stays under it.
   */
  async function setZoom(next: number, at?: { x: number; y: number }) {
    if (!stage || !canvas) return
    next = Math.min(maxZoom, Math.max(1, next))
    if (Math.abs(next - zoom) < 0.001) return
    const cx = at?.x ?? stage.clientWidth / 2
    const cy = at?.y ?? stage.clientHeight / 2
    const fx = (stage.scrollLeft + cx) / Math.max(canvas.offsetWidth, 1)
    const fy = (stage.scrollTop + cy) / Math.max(canvas.offsetHeight, 1)
    zoom = next
    await tick()
    if (!stage || !canvas) return
    stage.scrollLeft = fx * canvas.offsetWidth - cx
    stage.scrollTop = fy * canvas.offsetHeight - cy
  }

  const zoomBy = (factor: number, at?: { x: number; y: number }) => setZoom(zoom * factor, at)
  const fit = () => setZoom(1)
  const actualSize = () => setZoom(actualZoom <= 1.001 ? 2 : actualZoom)

  function viewportPoint(e: { clientX: number; clientY: number }) {
    const r = stage!.getBoundingClientRect()
    return { x: e.clientX - r.left, y: e.clientY - r.top }
  }

  // Ctrl/Cmd + wheel zooms, and so does a trackpad pinch, which browsers
  // report as a ctrl-wheel. A plain wheel scrolls the zoomed picture. Added
  // by hand because it has to be non-passive to stop the page zooming.
  $effect(() => {
    if (!stage) return
    const el = stage
    const onWheel = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return
      e.preventDefault()
      // A mouse wheel reports ~100 per notch, a pinch a few: clamp, so a
      // notch is a modest step either way.
      const dy = Math.max(-30, Math.min(30, e.deltaY))
      void zoomBy(Math.exp(-dy * 0.01), viewportPoint(e))
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  })

  function frameKeyDown(e: KeyboardEvent) {
    if (e.ctrlKey || e.metaKey || e.altKey) return
    if (e.key === '+' || e.key === '=') zoomBy(1.25)
    else if (e.key === '-' || e.key === '_') zoomBy(1 / 1.25)
    else if (e.key === '0') fit()
    else if (e.key === '1') actualSize()
    else return
    e.preventDefault()
  }

  // Dragging the zoomed picture pans it.
  let pan = $state<{ pointer: number; x: number; y: number; left: number; top: number } | null>(null)

  function panDown(e: PointerEvent) {
    if (!zoomed || e.button !== 0 || !stage) return
    stage.setPointerCapture(e.pointerId)
    pan = { pointer: e.pointerId, x: e.clientX, y: e.clientY, left: stage.scrollLeft, top: stage.scrollTop }
  }

  function panMove(e: PointerEvent) {
    if (!pan || e.pointerId !== pan.pointer || !stage) return
    stage.scrollLeft = pan.left - (e.clientX - pan.x)
    stage.scrollTop = pan.top - (e.clientY - pan.y)
  }

  function panEnd(e: PointerEvent) {
    if (pan && e.pointerId === pan.pointer) pan = null
  }

  // Asked for at the picture's on-screen size in device pixels, so it is
  // crisp on a HiDPI screen and, zoomed, shows real detail -- in 128-pixel
  // steps so resizing does not refetch on every pixel, and after a pause so
  // a wheel zoom asks once rather than at every step.
  const requestWidth = $derived(
    stageW > 0 ? Math.min(8192, Math.ceil((stageW * dpr()) / 128) * 128) : 0,
  )
  let fetchWidth = $state(0)
  $effect(() => {
    const w = requestWidth
    if (!w) return
    if (!untrack(() => fetchWidth)) {
      fetchWidth = w
      return
    }
    const t = window.setTimeout(() => (fetchWidth = w), 150)
    return () => window.clearTimeout(t)
  })

  // Double-buffered: the previous render stays up until the next one has
  // decoded, so editing never flashes a placeholder.
  let shown = $state('')
  let loadError = $state('')
  $effect(() => {
    if (!revision || !fetchWidth) return
    const path = api.previewImagePath(fetchWidth, revision)
    let cancelled = false
    let url = ''
    const controller = new AbortController()
    authorizedFetch(path, { signal: controller.signal })
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
      controller.abort()
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
    if (zoomed || e.button !== 0) return
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
    if (zoomed) return
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

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<div class="stage-wrap" onkeydown={frameKeyDown} role="group" aria-label="Preview with zoom">
  <div class="frame" style:--ratio={pictureRatio}>
    <div class="zoom-bar" role="toolbar" aria-label="Zoom">
      <button type="button" onclick={() => zoomBy(1 / 1.25)} disabled={!zoomed} aria-label="Zoom out" title="Zoom out (−)">
        <Icon name="zoom-out" size={15} />
      </button>
      <span class="level" aria-live="polite" title="Size relative to the picture's real pixels">{percent}%</span>
      <button type="button" onclick={() => zoomBy(1.25)} disabled={zoom >= maxZoom - 0.001} aria-label="Zoom in" title="Zoom in (+, or Ctrl/⌘ + scroll, or pinch)">
        <Icon name="zoom-in" size={15} />
      </button>
      <button type="button" onclick={fit} disabled={!zoomed} title="Fit the whole picture (0)">Fit</button>
      <button type="button" onclick={actualSize} title="Real pixels, one picture pixel per screen pixel (1)">100%</button>
    </div>
    {#if zoomed}
      <p class="zoom-note" role="status">Zoomed: drag to pan. Layers are locked until you fit the picture.</p>
    {/if}

    <!-- Focusable only while zoomed, so the arrow keys can scroll it. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <div
      class="stage"
      class:busy
      class:zoomed
      class:panning={pan !== null}
      bind:this={stage}
      style:aspect-ratio={`${pictureWidth || 16} / ${pictureHeight || 9}`}
      style:--ratio={pictureRatio}
      role="application"
      aria-label="Watermark preview. Select a layer and drag it, or use the arrow keys to move it. Plus and minus zoom."
      tabindex={zoomed ? 0 : -1}
      onpointerdown={(e) => {
        if (zoomed) panDown(e)
        else if (e.target === stage || e.target === canvas || (e.target as HTMLElement).tagName === 'IMG') onSelect(-1)
      }}
      onpointermove={panMove}
      onpointerup={panEnd}
      onpointercancel={panEnd}
    >
      <div class="canvas" bind:this={canvas} style:width={`${zoom * 100}%`} style:aspect-ratio={`${pictureWidth || 16} / ${pictureHeight || 9}`}>
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
          class:locked={zoomed}
          role="button"
          tabindex={zoomed ? -1 : 0}
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
  </div>
</div>

<style>
  .stage-wrap {
    width: 100%;
    display: flex;
    justify-content: center;
  }

  .frame {
    position: relative;
    width: 100%;
    max-width: calc(72vh * var(--ratio, 1.78));
  }

  .zoom-bar {
    position: absolute;
    z-index: 5;
    top: 8px;
    right: 8px;
    display: flex;
    align-items: center;
    gap: 2px;
    padding: 3px;
    background: rgba(9, 9, 11, 0.78);
    border-radius: var(--radius-md);
    color: #fff;
  }

  .zoom-bar button {
    display: inline-flex;
    align-items: center;
    padding: 0.2rem 0.45rem;
    font-size: var(--text-xs);
    color: #fff;
    background: transparent;
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
  }

  .zoom-bar button:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.18);
  }

  .zoom-bar button:disabled {
    opacity: 0.4;
  }

  .zoom-bar .level {
    min-width: 3rem;
    text-align: center;
    font-size: var(--text-xs);
    font-variant-numeric: tabular-nums;
  }

  .zoom-note {
    position: absolute;
    z-index: 5;
    left: 8px;
    bottom: 8px;
    margin: 0;
    padding: 0.2rem 0.6rem;
    font-size: var(--text-xs);
    color: #fff;
    background: rgba(9, 9, 11, 0.78);
    border-radius: var(--radius-pill);
    pointer-events: none;
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

  /* Zoomed, the stage scrolls and a drag pans it. A flat background and no
     shadow on the picture keep panning a 4K+ bitmap cheap. */
  .stage.zoomed {
    overflow: auto;
    cursor: grab;
    overscroll-behavior: contain;
  }

  .stage.zoomed.panning {
    cursor: grabbing;
  }

  /* The picture itself: layer geometry is measured against this, and it is
     as wide as the zoom says while the stage stays the size of the fit. */
  .canvas {
    position: relative;
    max-width: none;
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

  /* Zoomed: still drawn, so the selection is visible, but not grabbable. */
  .handle.locked {
    pointer-events: none;
    cursor: inherit;
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
