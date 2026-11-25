<script lang="ts">
  // Every property of the selected layer, grouped. Values are the layer's own
  // picture-relative units; the readouts beside them ("≈ 21 px", "5 % from
  // the right") translate them for this picture without changing what is
  // stored.

  import type { FontInfo, LayerResult, TemplateHelp, UnsupportedFont, WatermarkLayer } from '../api'
  import ColorInput from './ColorInput.svelte'
  import FontPicker from './FontPicker.svelte'
  import NumberSlider from './NumberSlider.svelte'
  import TemplateEditor from './TemplateEditor.svelte'
  import { ALIGNMENTS, ANCHORS, marginOf, reanchor, round4, setMargin } from './model'

  let {
    layer = $bindable(),
    index,
    result,
    fonts,
    unsupportedFonts = [],
    help,
    onEdit,
  }: {
    layer: WatermarkLayer
    index: number
    result?: LayerResult
    fonts: FontInfo[]
    unsupportedFonts?: UnsupportedFont[]
    help: TemplateHelp | null
    /** Records an undo step; edits sharing a key coalesce into one. */
    onEdit: (key?: string) => void
  } = $props()

  let linkedPadding = $state(true)

  const marginX = $derived(marginOf(layer, 'x'))
  const marginY = $derived(marginOf(layer, 'y'))
  const edgeX = $derived(layer.anchor.endsWith('left') ? 'left' : 'right')
  const edgeY = $derived(layer.anchor.startsWith('top') ? 'top' : 'bottom')

  function setAnchor(anchor: string) {
    onEdit()
    // Keeps the text where it is: only the point x and y describe changes.
    reanchor(layer, anchor, result?.textBounds)
  }

  function setPadding(side: number, v: number) {
    onEdit('padding')
    const p = layer.background.padding
    if (linkedPadding) {
      // Linked: vertical sides together, horizontal sides together.
      if (side % 2 === 0) p[0] = p[2] = v
      else p[1] = p[3] = v
    } else {
      p[side] = v
    }
  }

  const sides = ['Top', 'Right', 'Bottom', 'Left']
  const px = (em: number) => (result?.fontPx ? `≈ ${Math.round(em * result.fontPx)} px` : '')
</script>

<div class="inspector">
  <details open>
    <summary>Text</summary>
    <div class="group">
      <label class="row-label" for="wm-name">Name</label>
      <input
        id="wm-name"
        type="text"
        bind:value={layer.name}
        placeholder={`Layer ${index + 1}`}
        onbeforeinput={() => onEdit('name')}
      />

      <span class="row-label">Template</span>
      <TemplateEditor bind:value={layer.template} {help} {result} onEdit={() => onEdit('template')} />

      <span class="row-label">Alignment</span>
      <div class="segmented" role="radiogroup" aria-label="Line alignment">
        {#each ALIGNMENTS as a (a)}
          <button
            type="button"
            role="radio"
            aria-checked={layer.alignment === a}
            class:on={layer.alignment === a}
            onclick={() => {
              onEdit()
              layer.alignment = a
            }}
          >{a}</button>
        {/each}
      </div>

      <NumberSlider
        bind:value={layer.linespace}
        id="wm-linespace"
        label="Line spacing"
        min={0.5}
        max={4}
        step={0.05}
        unit="×"
        {onEdit}
      />
    </div>
  </details>

  <details open>
    <summary>Font</summary>
    <div class="group">
      <span class="row-label">Family</span>
      <FontPicker bind:value={layer.font} {fonts} unsupported={unsupportedFonts} id="wm-font" onEdit={() => onEdit()} />
      {#if result?.error?.stage === 'font'}
        <p class="error small" role="alert">{result.error.message}</p>
      {/if}

      <NumberSlider
        bind:value={layer.size}
        id="wm-size"
        label="Size"
        min={0.2}
        max={15}
        step={0.1}
        unit="%"
        hint={`of the picture's shorter side${result?.fontPx ? ` · ≈ ${Math.round(result.fontPx)} px on this picture` : ''}`}
        {onEdit}
      />

      <span class="row-label">Colour</span>
      <ColorInput bind:value={layer.color} label="Text" id="wm-color" {onEdit} />
    </div>
  </details>

  <details open>
    <summary>Position</summary>
    <div class="group">
      <span class="row-label">Anchor</span>
      <div class="anchor-row">
        <div class="anchor-grid" role="radiogroup" aria-label="Anchor: which point of the text the position places">
          {#each ANCHORS as a (a)}
            <button
              type="button"
              role="radio"
              aria-checked={layer.anchor === a}
              aria-label={a}
              title={a}
              class:on={layer.anchor === a}
              onclick={() => setAnchor(a)}
            ><span></span></button>
          {/each}
        </div>
        <p class="muted small">
          The point of the text box that X and Y place. Changing it keeps the text
          where it is. Drag the text on the preview to move it.
        </p>
      </div>

      <NumberSlider bind:value={layer.x} id="wm-x" label="X" min={-0.5} max={1.5} step={0.001} scale={100} unit="%" hint="of the picture's width, from the left" {onEdit} />
      <NumberSlider bind:value={layer.y} id="wm-y" label="Y" min={-0.5} max={1.5} step={0.001} scale={100} unit="%" hint="of the picture's height, from the top" {onEdit} />

      {#if marginX !== null || marginY !== null}
        <div class="margins">
          {#if marginX !== null}
            <label>
              <span class="muted">Margin from {edgeX}</span>
              <input
                type="number"
                step="0.1"
                value={round4(marginX * 100)}
                onchange={(e) => {
                  onEdit()
                  setMargin(layer, 'x', Number((e.currentTarget as HTMLInputElement).value) / 100)
                }}
              /> %
            </label>
          {/if}
          {#if marginY !== null}
            <label>
              <span class="muted">Margin from {edgeY}</span>
              <input
                type="number"
                step="0.1"
                value={round4(marginY * 100)}
                onchange={(e) => {
                  onEdit()
                  setMargin(layer, 'y', Number((e.currentTarget as HTMLInputElement).value) / 100)
                }}
              /> %
            </label>
          {/if}
        </div>
      {/if}
    </div>
  </details>

  <details open={Boolean(layer.background.color)}>
    <summary>Background box</summary>
    <div class="group">
      <span class="row-label">Colour</span>
      <ColorInput bind:value={layer.background.color} label="Background" id="wm-bg" optional {onEdit} />

      {#if layer.background.color}
        <div class="checks">
          <label>
            <input type="checkbox" checked={layer.background.hThroughout} onchange={() => {
              onEdit()
              layer.background.hThroughout = !layer.background.hThroughout
            }} /> Full width
          </label>
          <label>
            <input type="checkbox" checked={layer.background.vThroughout} onchange={() => {
              onEdit()
              layer.background.vThroughout = !layer.background.vThroughout
            }} /> Full height
          </label>
          <label>
            <input type="checkbox" bind:checked={linkedPadding} /> Link paddings
          </label>
        </div>
        {#each sides as side, i (side)}
          {#if !linkedPadding || i < 2}
            <NumberSlider
              id={`wm-pad-${i}`}
              label={linkedPadding ? (i === 0 ? 'Vertical' : 'Horizontal') : side}
              min={0}
              max={6}
              step={0.05}
              unit="em"
              hint={px(layer.background.padding[i])}
              onEdit={() => {}}
              bind:value={
                () => layer.background.padding[i],
                (v) => setPadding(i, v)
              }
            />
          {/if}
        {/each}
      {/if}
    </div>
  </details>

  <details open={Boolean(layer.shadow.color || layer.outline.color)}>
    <summary>Effects</summary>
    <div class="group">
      <span class="row-label">Shadow</span>
      <ColorInput bind:value={layer.shadow.color} label="Shadow" id="wm-shadow" optional {onEdit} />
      {#if layer.shadow.color}
        <NumberSlider bind:value={layer.shadow.dx} id="wm-shadow-dx" label="Offset X" min={-1} max={1} step={0.01} unit="em" hint={px(layer.shadow.dx)} {onEdit} />
        <NumberSlider bind:value={layer.shadow.dy} id="wm-shadow-dy" label="Offset Y" min={-1} max={1} step={0.01} unit="em" hint={px(layer.shadow.dy)} {onEdit} />
      {/if}

      <span class="row-label">Outline</span>
      <ColorInput
        bind:value={
          () => layer.outline.color,
          (v) => {
            layer.outline.color = v
            if (v && !layer.outline.width) layer.outline.width = 0.04
          }
        }
        label="Outline"
        id="wm-outline"
        optional
        {onEdit}
      />
      {#if layer.outline.color}
        <NumberSlider bind:value={layer.outline.width} id="wm-outline-w" label="Width" min={0} max={0.3} step={0.005} unit="em" hint={px(layer.outline.width)} {onEdit} />
      {/if}
    </div>
  </details>
</div>

<style>
  .inspector {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  details {
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  summary {
    cursor: pointer;
    padding: var(--space-2) var(--space-3);
    font-weight: 600;
    font-size: var(--text-sm);
    user-select: none;
  }

  .group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: 0 var(--space-3) var(--space-3);
  }

  .row-label {
    font-size: var(--text-xs);
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted);
    margin-top: var(--space-1);
  }

  .segmented {
    display: inline-flex;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    overflow: hidden;
    align-self: flex-start;
  }

  .segmented button {
    border: none;
    border-radius: 0;
    padding: 0.25rem 0.75rem;
    text-transform: capitalize;
  }

  .segmented button.on {
    background: var(--accent);
    color: var(--accent-fg);
  }

  .anchor-row {
    display: flex;
    gap: var(--space-3);
    align-items: center;
  }

  .anchor-grid {
    display: grid;
    grid-template-columns: repeat(3, 1.5rem);
    gap: 3px;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: var(--radius-sm);
    background: var(--surface-subtle);
    flex-shrink: 0;
  }

  .anchor-grid button {
    width: 1.5rem;
    height: 1.5rem;
    padding: 0;
    display: grid;
    place-items: center;
    border-radius: 3px;
  }

  .anchor-grid button span {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: var(--border-strong);
  }

  .anchor-grid button.on {
    background: var(--accent);
    border-color: var(--accent);
  }

  .anchor-grid button.on span {
    background: var(--accent-fg);
  }

  .margins {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }

  .margins input {
    width: 4.6rem;
  }

  .margins label {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }

  .checks {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-sm);
  }

  .small {
    font-size: var(--text-xs);
    margin: 0;
  }
</style>
