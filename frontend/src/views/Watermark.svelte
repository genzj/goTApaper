<script lang="ts">
  // The watermark editor.
  //
  // Two separate actions, deliberately:
  //   * Preview renders the draft onto a captured copy of the current
  //     wallpaper, into one file. The stage shows that file, and "Preview on
  //     desktop" sets the very same file as the wallpaper until "Revert" --
  //     never touching the real wallpaper file or the history.
  //   * Save stores the layers. They apply from the next refresh; saving does
  //     not change the desktop.

  import { onDestroy, untrack } from 'svelte'
  import {
    api,
    ApiError,
    type DownloadStatus,
    type FontInfo,
    type FractionRect,
    type PreviewBase,
    type PreviewResult,
    type TemplateHelp,
    type UnsupportedFont,
    type WatermarkLayer,
    type WatermarkPreset,
  } from '../lib/api'
  import { configVersion, lastDownload } from '../lib/events'
  import Icon from '../lib/Icon.svelte'
  import { Draft } from '../lib/watermark/draft.svelte'
  import Inspector from '../lib/watermark/Inspector.svelte'
  import LayerList from '../lib/watermark/LayerList.svelte'
  import { ASPECT_GUIDES, clamp, cloneLayer, newLayer, placeFromBox, round4 } from '../lib/watermark/model'
  import PresetPicker from '../lib/watermark/PresetPicker.svelte'
  import Stage from '../lib/watermark/Stage.svelte'

  const STORAGE_KEY = 'gotapaper:watermark-draft'

  const draft = new Draft()
  let loading = $state(true)
  let loadError = $state('')
  let fonts = $state<FontInfo[]>([])
  let unsupportedFonts = $state<UnsupportedFont[]>([])
  let presets = $state<WatermarkPreset[]>([])
  let help = $state<TemplateHelp | null>(null)
  let base = $state<PreviewBase | null>(null)
  let noCapture = $state('')
  let result = $state<PreviewResult | null>(null)
  let rendering = $state(false)
  let renderError = $state('')
  let debug = $state(false)
  let guides = $state<string[]>([])
  let showPresets = $state(false)
  let saving = $state(false)
  let notice = $state<{ kind: 'info' | 'error'; text: string; refresh?: boolean } | null>(null)
  let status = $state<DownloadStatus | null>(null)
  let actionBusy = $state(false)

  const defaultFont = $derived(presets[0]?.layers[0]?.font ?? fonts[0]?.file ?? 'NotoSans-Regular.ttf')
  const selectedLayer = $derived(draft.layers[draft.selected])
  const selectedResult = $derived(result?.layers[draft.selected])
  const previewing = $derived(Boolean(status?.previewing))
  const desktopBehind = $derived(previewing && result !== null && status?.previewRevision !== result.revision)

  function message(e: unknown): string {
    return e instanceof Error ? e.message : String(e)
  }

  async function load() {
    loading = true
    loadError = ''
    try {
      const [layers, f, u, p] = await Promise.all([
        api.watermarkLayers(),
        api.fonts(),
        api.unsupportedFonts().catch(() => []),
        api.watermarkPresets(),
      ])
      fonts = f
      unsupportedFonts = u
      presets = p
      draft.reset(layers)
      restoreUnsaved(layers)
      await loadBase()
      await refreshStatus()
    } catch (e) {
      loadError = message(e)
    } finally {
      loading = false
    }
  }

  async function loadBase() {
    try {
      base = await api.previewBase()
      noCapture = ''
    } catch (e) {
      base = null
      noCapture = message(e)
    }
    // After the capture, so each field carries its value on this picture.
    help = await api.templateHelp().catch(() => help)
  }

  async function recapture() {
    try {
      base = await api.capturePreview()
      noCapture = ''
      help = await api.templateHelp().catch(() => help)
      scheduleRender(0)
    } catch (e) {
      notice = { kind: 'error', text: message(e) }
    }
  }

  async function refreshStatus() {
    status = (await api.status().catch(() => null))?.download ?? status
  }

  // Unsaved edits survive leaving the tab or reloading the window.
  function restoreUnsaved(serverLayers: WatermarkLayer[]) {
    try {
      const raw = sessionStorage.getItem(STORAGE_KEY)
      if (!raw) return
      const stored = JSON.parse(raw) as { saved: string; layers: WatermarkLayer[] }
      // Only if the saved layers have not changed underneath since.
      if (stored.saved !== JSON.stringify(serverLayers)) return
      if (JSON.stringify(stored.layers) === stored.saved) return
      draft.commit()
      draft.layers = stored.layers
      notice = { kind: 'info', text: 'Restored your unsaved changes.' }
    } catch {
      // Ignore a corrupt or inaccessible session store.
    }
  }

  $effect(() => {
    const snapshot = JSON.stringify(draft.layers)
    try {
      if (draft.dirty) sessionStorage.setItem(STORAGE_KEY, JSON.stringify({ saved: draft.saved, layers: JSON.parse(snapshot) }))
      else sessionStorage.removeItem(STORAGE_KEY)
    } catch {
      // Storage may be unavailable; the draft still works.
    }
  })

  // ---- rendering ---------------------------------------------------------

  let timer: number | undefined
  let inflight: AbortController | null = null

  function scheduleRender(delay = 150) {
    window.clearTimeout(timer)
    timer = window.setTimeout(render, delay)
  }

  async function render() {
    if (!base) return
    inflight?.abort()
    const controller = new AbortController()
    inflight = controller
    rendering = true
    try {
      const res = await api.renderPreview(draft.forSave(), debug, controller.signal)
      if (inflight !== controller) return
      result = res
      renderError = ''
      if (res.base.stale !== base.stale) base = res.base
    } catch (e) {
      if (controller.signal.aborted) return
      if (e instanceof ApiError && e.status === 409 && /superseded/.test(e.message)) return
      renderError = message(e)
    } finally {
      if (inflight === controller) {
        rendering = false
        inflight = null
      }
    }
  }

  // Any change to the layers or the debug switch renders again, debounced.
  // Depends on whether there is a capture, not on the capture object itself:
  // render() refreshes `base` from each result, and tracking the object would
  // make every render schedule the next one.
  const hasBase = $derived(base !== null)
  $effect(() => {
    void JSON.stringify(draft.layers)
    void debug
    if (hasBase) untrack(() => scheduleRender())
  })

  // A finished job -- a refresh, a desktop preview, a revert -- changes what
  // the desktop shows and may make the capture stale.
  // Only the event is tracked. Reading `base` here while assigning it from the
  // response made each answer re-run the effect: an endless status/base loop.
  $effect(() => {
    void $lastDownload
    untrack(() => {
      void refreshStatus()
      if (base) void api.previewBase().then((b) => (base = b)).catch(() => {})
    })
  })

  // Saved elsewhere -- another window, the CLI -- while nothing is edited
  // here: follow it.
  $effect(() => {
    void $configVersion
    untrack(() => {
      if (loading || draft.dirty) return
      api.watermarkLayers().then((layers) => {
        if (!draft.dirty && JSON.stringify(layers) !== draft.saved) draft.reset(layers)
      }).catch(() => {})
    })
  })

  onDestroy(() => {
    window.clearTimeout(timer)
    inflight?.abort()
  })

  // ---- layer operations --------------------------------------------------

  function edit(key?: string) {
    draft.commit(key)
  }

  function addLayer() {
    edit()
    const l = newLayer(defaultFont)
    // Stacked above the last one rather than exactly on it.
    l.y = round4(clamp(0.95 - 0.04 * draft.layers.length, 0.05, 0.95))
    draft.layers.push(l)
    draft.selected = draft.layers.length - 1
  }

  function duplicate(i: number) {
    edit()
    const copy = cloneLayer(draft.layers[i])
    copy.id = 0
    copy.name = copy.name ? `${copy.name} copy` : ''
    copy.y = round4(clamp(copy.y - 0.03, 0, 1))
    draft.layers.splice(i + 1, 0, copy)
    draft.selected = i + 1
  }

  function remove(i: number) {
    edit()
    draft.layers.splice(i, 1)
    draft.selected = clamp(draft.selected > i ? draft.selected - 1 : draft.selected, 0, Math.max(0, draft.layers.length - 1))
  }

  function toggle(i: number) {
    edit()
    draft.layers[i].enabled = !draft.layers[i].enabled
  }

  function move(from: number, to: number) {
    edit()
    const [l] = draft.layers.splice(from, 1)
    draft.layers.splice(to, 0, l)
    draft.selected = to
  }

  function drop(i: number, box: FractionRect) {
    edit()
    const place = placeFromBox(box)
    const l = draft.layers[i]
    l.anchor = place.anchor
    l.x = place.x
    l.y = place.y
  }

  function nudge(i: number, dx: number, dy: number) {
    edit('nudge')
    const l = draft.layers[i]
    l.x = round4(clamp(l.x + dx, -0.5, 1.5))
    l.y = round4(clamp(l.y + dy, -0.5, 1.5))
  }

  // A preset is added as new layers on top, never in place of the user's
  // own: picking one must not discard what they have built.
  function pickPreset(p: WatermarkPreset) {
    edit()
    const added = p.layers.map((l) => ({ ...cloneLayer(l), id: 0 }))
    draft.layers.push(...added)
    draft.selected = draft.layers.length - 1
    showPresets = false
    notice = {
      kind: 'info',
      text: `Added “${p.name}” as ${added.length > 1 ? `${added.length} new layers` : 'a new layer'} on top. Undo removes it.`,
    }
  }

  // ---- save, reset, desktop ----------------------------------------------

  async function save() {
    saving = true
    try {
      const saved = await api.replaceWatermarkLayers(draft.forSave())
      draft.markSaved(saved)
      notice = { kind: 'info', text: 'Saved. The layers apply from the next refresh.', refresh: true }
    } catch (e) {
      notice = { kind: 'error', text: message(e) }
    } finally {
      saving = false
    }
  }

  async function reset() {
    if (draft.dirty && !confirm('Discard your unsaved changes to the watermark layers?')) return
    try {
      draft.reset(await api.watermarkLayers())
      notice = null
    } catch (e) {
      notice = { kind: 'error', text: message(e) }
    }
  }

  async function refreshNow() {
    try {
      await api.refresh([], true)
      notice = { kind: 'info', text: 'Refreshing; the new wallpaper will carry the saved layers.' }
    } catch (e) {
      notice = { kind: 'error', text: message(e) }
    }
  }

  async function desktop(action: 'show' | 'revert') {
    actionBusy = true
    try {
      if (action === 'show') {
        // Render first if a change is still waiting, so the desktop gets
        // exactly what the stage will show.
        if (timer !== undefined) {
          window.clearTimeout(timer)
          await render()
        }
        status = await api.showPreviewOnDesktop()
      } else {
        status = await api.revertPreview()
      }
    } catch (e) {
      notice = { kind: 'error', text: message(e) }
    } finally {
      actionBusy = false
      // The job runs in the background; its end arrives as an event, but a
      // fast one may finish before this view subscribes to it.
      window.setTimeout(refreshStatus, 400)
    }
  }

  function keydown(e: KeyboardEvent) {
    const target = e.target as HTMLElement
    const typing = /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)
    const mod = e.ctrlKey || e.metaKey
    if (mod && e.key.toLowerCase() === 's') {
      e.preventDefault()
      if (draft.dirty && !saving) void save()
      return
    }
    if (typing) return
    if (mod && e.key.toLowerCase() === 'z') {
      e.preventDefault()
      if (e.shiftKey) draft.redo()
      else draft.undo()
    } else if (mod && e.key.toLowerCase() === 'y') {
      e.preventDefault()
      draft.redo()
    } else if ((e.key === 'Delete' || e.key === 'Backspace') && target.classList.contains('handle')) {
      e.preventDefault()
      remove(draft.selected)
    }
  }

  function beforeUnload(e: BeforeUnloadEvent) {
    if (draft.dirty) e.preventDefault()
  }

  void load()
</script>

<svelte:window onkeydown={keydown} onbeforeunload={beforeUnload} />

{#if loading}
  <div class="empty-state"><p class="muted">Loading the watermark editor…</p></div>
{:else if loadError}
  <div class="banner" role="alert">{loadError}</div>
  <button type="button" onclick={load}>Try again</button>
{:else}
  <div class="toolbar">
    <div class="row">
      <button type="button" onclick={() => (showPresets = true)} disabled={!presets.length}>
        <Icon name="sparkles" size={15} /> Presets
      </button>
      <span class="divider"></span>
      <button type="button" onclick={() => draft.undo()} disabled={!draft.canUndo} aria-label="Undo" title="Undo (Ctrl+Z)">
        <Icon name="undo" size={15} />
      </button>
      <button type="button" onclick={() => draft.redo()} disabled={!draft.canRedo} aria-label="Redo" title="Redo (Ctrl+Shift+Z)">
        <Icon name="redo" size={15} />
      </button>
      <span class="divider"></span>
      <label class="toggle" title="Draw each layer's bounds, background box and anchor, from the renderer itself">
        <input type="checkbox" bind:checked={debug} /> Debug overlay
      </label>
      <details class="guides">
        <summary>Aspect guides{guides.length ? ` (${guides.length})` : ''}</summary>
        <div class="guides-menu">
          <p class="muted small">What a display of each shape shows when the wallpaper fills it.</p>
          {#each ASPECT_GUIDES as g (g.label)}
            <label>
              <input type="checkbox" checked={guides.includes(g.label)} onchange={() => {
                guides = guides.includes(g.label) ? guides.filter((x) => x !== g.label) : [...guides, g.label]
              }} /> {g.label}
            </label>
          {/each}
        </div>
      </details>
    </div>
    <div class="row">
      {#if previewing}
        <button type="button" onclick={() => desktop('revert')} disabled={actionBusy}>
          <Icon name="restart" size={15} /> Revert desktop
        </button>
      {/if}
      <button
        type="button"
        onclick={() => desktop('show')}
        disabled={actionBusy || !result}
        class:btn-primary={desktopBehind}
        title="Show this preview as the wallpaper until you revert. The real wallpaper file is not changed."
      >
        <Icon name="monitor" size={15} /> {previewing ? 'Update desktop' : 'Preview on desktop'}
      </button>
      <span class="divider"></span>
      <button type="button" onclick={reset} disabled={!draft.dirty}>Reset</button>
      <button type="button" class="btn-primary" onclick={save} disabled={!draft.dirty || saving} title="Save (Ctrl+S). Applies from the next refresh.">
        <Icon name="check" size={15} /> {saving ? 'Saving…' : 'Save'}
      </button>
    </div>
  </div>

  {#if previewing}
    <div class="note warn-note" role="status">
      <Icon name="monitor" size={15} />
      Your desktop is showing a watermark preview, not the real wallpaper.
      {#if desktopBehind}It is behind your latest edits — <button type="button" class="link" onclick={() => desktop('show')}>update it</button>.{/if}
      <button type="button" class="link" onclick={() => desktop('revert')}>Revert</button>
    </div>
  {/if}
  {#if notice}
    <div class="note" class:error-note={notice.kind === 'error'} role={notice.kind === 'error' ? 'alert' : 'status'}>
      {notice.text}
      {#if notice.refresh}
        <button type="button" class="link" onclick={refreshNow}>Refresh now</button>
      {/if}
      <button type="button" class="link dismiss" onclick={() => (notice = null)} aria-label="Dismiss">✕</button>
    </div>
  {/if}
  {#if base?.stale}
    <div class="note" role="status">
      The wallpaper has changed since this picture was captured.
      <button type="button" class="link" onclick={recapture}>Preview on the new one</button>
    </div>
  {/if}

  <div class="editor">
    <section class="stage-col" aria-label="Preview">
      {#if noCapture}
        <div class="empty-state">
          <p>{noCapture}</p>
          <button type="button" onclick={recapture}>Try again</button>
        </div>
      {:else if base}
        <Stage
          revision={result?.revision ?? 0}
          pictureWidth={base.width}
          pictureHeight={base.height}
          layers={draft.layers}
          results={result?.layers ?? []}
          selected={draft.selected}
          busy={rendering}
          {guides}
          onSelect={(i) => {
            if (i >= 0) draft.selected = i
          }}
          onDrop={drop}
          onNudge={nudge}
        />
        <p class="caption muted">
          {base.title ?? 'Current wallpaper'} · {base.width} × {base.height}
          {#if rendering}· rendering…{/if}
          {#if renderError}<span class="error">· {renderError}</span>{/if}
          <span class="tips">Drag text to move it (Alt: no snapping) · arrows nudge, Shift ×10</span>
        </p>
      {/if}
    </section>

    <aside class="side-col">
      <LayerList
        layers={draft.layers}
        results={result?.layers ?? []}
        selected={draft.selected}
        onSelect={(i) => (draft.selected = i)}
        onToggle={toggle}
        onDuplicate={duplicate}
        onDelete={remove}
        onMove={move}
        onAdd={addLayer}
      />
      {#if selectedLayer}
        {#key draft.selected}
          <Inspector
            bind:layer={draft.layers[draft.selected]}
            index={draft.selected}
            result={selectedResult}
            {fonts}
            {unsupportedFonts}
            {help}
            onEdit={edit}
          />
        {/key}
      {/if}
    </aside>
  </div>

  {#if showPresets}
    <PresetPicker {presets} onPick={pickPreset} onClose={() => (showPresets = false)} />
  {/if}
{/if}

<style>
  .divider {
    width: 1px;
    align-self: stretch;
    background: var(--border-subtle);
  }

  .toolbar button,
  .toolbar .toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.35rem;
  }

  .toggle {
    font-size: var(--text-sm);
  }

  .guides {
    position: relative;
    font-size: var(--text-sm);
  }

  .guides summary {
    cursor: pointer;
    list-style: none;
    padding: 0.3rem 0.6rem;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
  }

  .guides-menu {
    position: absolute;
    z-index: 30;
    top: calc(100% + 4px);
    left: 0;
    width: 14rem;
    padding: var(--space-2) var(--space-3);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .small {
    font-size: var(--text-xs);
    margin: 0 0 var(--space-1);
  }

  .note {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    margin-bottom: var(--space-3);
    border: 1px solid var(--accent-border);
    background: var(--accent-surface);
    border-radius: var(--radius-md);
    font-size: var(--text-sm);
  }

  .warn-note {
    border-color: var(--warn-border);
    background: var(--warn-surface);
    color: var(--warn);
  }

  .error-note {
    border-color: var(--error-border);
    background: var(--error-surface);
    color: var(--error);
  }

  .link {
    border: none;
    background: none;
    padding: 0;
    color: var(--accent);
    font-weight: 600;
    cursor: pointer;
    text-decoration: underline;
  }

  .dismiss {
    margin-left: auto;
    text-decoration: none;
    color: var(--muted);
  }

  .editor {
    display: grid;
    grid-template-columns: minmax(0, 1fr) 24rem;
    gap: var(--space-4);
    align-items: start;
  }

  .stage-col {
    position: sticky;
    top: calc(var(--header-height) + var(--space-3));
    min-width: 0;
  }

  .side-col {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .caption {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-2);
    margin: var(--space-2) 0 0;
    font-size: var(--text-xs);
  }

  .tips {
    margin-left: auto;
  }

  @media (max-width: 1100px) {
    .editor {
      grid-template-columns: 1fr;
    }

    .stage-col {
      position: static;
    }
  }
</style>
