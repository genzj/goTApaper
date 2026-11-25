<script lang="ts">
  // The layer list, top layer first as in any layer panel: show/hide,
  // rename, duplicate, delete and drag to reorder. Index 0 of the draft is
  // drawn first (bottom), so the list shows it reversed.

  import type { LayerResult, WatermarkLayer } from '../api'
  import Icon from '../Icon.svelte'
  import { layerLabel } from './model'

  let {
    layers,
    results,
    selected,
    onSelect,
    onToggle,
    onDuplicate,
    onDelete,
    onMove,
    onAdd,
  }: {
    layers: WatermarkLayer[]
    results: LayerResult[]
    selected: number
    onSelect: (index: number) => void
    onToggle: (index: number) => void
    onDuplicate: (index: number) => void
    onDelete: (index: number) => void
    /** Moves a layer from one draft index to another. */
    onMove: (from: number, to: number) => void
    onAdd: () => void
  } = $props()

  // Rows in display order: the topmost layer first.
  const rows = $derived(layers.map((l, i) => ({ l, i })).reverse())

  let dragFrom = $state<number | null>(null)
  let dragOver = $state<number | null>(null)
  let list = $state<HTMLOListElement>()

  function rowAt(clientY: number): number | null {
    if (!list) return null
    const items = [...list.querySelectorAll<HTMLElement>('[data-index]')]
    for (const el of items) {
      const r = el.getBoundingClientRect()
      if (clientY < r.top + r.height / 2) return Number(el.dataset.index)
    }
    const last = items.at(-1)
    return last ? Number(last.dataset.index) : null
  }

  function gripDown(e: PointerEvent, index: number) {
    ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
    dragFrom = index
    dragOver = index
  }

  function gripMove(e: PointerEvent) {
    if (dragFrom === null) return
    dragOver = rowAt(e.clientY)
  }

  function gripUp() {
    if (dragFrom !== null && dragOver !== null && dragOver !== dragFrom) onMove(dragFrom, dragOver)
    dragFrom = dragOver = null
  }

  function gripKey(e: KeyboardEvent, index: number) {
    // Up in the list is towards the top layer: a higher draft index.
    if (e.key === 'ArrowUp' && index < layers.length - 1) {
      e.preventDefault()
      onMove(index, index + 1)
    } else if (e.key === 'ArrowDown' && index > 0) {
      e.preventDefault()
      onMove(index, index - 1)
    }
  }
</script>

<div class="layer-list">
  <div class="head">
    <h3>Layers</h3>
    <button type="button" onclick={onAdd} disabled={layers.length >= 32}>
      <Icon name="plus" size={14} /> Add
    </button>
  </div>

  {#if layers.length === 0}
    <p class="muted empty">No layers. Add one, or start from a preset.</p>
  {:else}
    <ol bind:this={list}>
      {#each rows as { l, i } (i)}
        {@const r = results[i]}
        <li
          data-index={i}
          class:selected={selected === i}
          class:off={!l.enabled}
          class:drop-target={dragOver === i && dragFrom !== null && dragFrom !== i}
          class:dragging={dragFrom === i}
        >
          <button
            type="button"
            class="grip"
            aria-label={`Reorder ${layerLabel(l, i)}: drag, or use the arrow keys`}
            onpointerdown={(e) => gripDown(e, i)}
            onpointermove={gripMove}
            onpointerup={gripUp}
            onpointercancel={gripUp}
            onkeydown={(e) => gripKey(e, i)}
          ><Icon name="grip" size={14} /></button>
          <button
            type="button"
            class="eye"
            aria-pressed={l.enabled}
            aria-label={l.enabled ? `Hide ${layerLabel(l, i)}` : `Show ${layerLabel(l, i)}`}
            onclick={() => onToggle(i)}
          ><Icon name={l.enabled ? 'eye' : 'eye-off'} size={15} /></button>
          <button type="button" class="label" onclick={() => onSelect(i)} aria-current={selected === i}>
            <span class="name">{layerLabel(l, i)}</span>
            <span class="sub">
              {#if r?.error}
                <span class="error">{r.error.stage} error</span>
              {:else if r?.text}
                {r.text.split('\n')[0]}
              {:else}
                <span class="muted mono">{l.template || '(empty)'}</span>
              {/if}
            </span>
          </button>
          <button type="button" class="icon-btn" aria-label={`Duplicate ${layerLabel(l, i)}`} title="Duplicate" onclick={() => onDuplicate(i)}>
            <Icon name="copy" size={14} />
          </button>
          <button type="button" class="icon-btn btn-danger" aria-label={`Delete ${layerLabel(l, i)}`} title="Delete" onclick={() => onDelete(i)}>
            <Icon name="trash" size={14} />
          </button>
        </li>
      {/each}
    </ol>
  {/if}
</div>

<style>
  .layer-list {
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  h3 {
    margin: 0;
    font-size: var(--text-sm);
  }

  .head button {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
  }

  .empty {
    padding: var(--space-3);
    margin: 0;
    font-size: var(--text-sm);
  }

  ol {
    list-style: none;
    margin: 0;
    padding: var(--space-1);
    max-height: 18rem;
    overflow: auto;
  }

  li {
    display: flex;
    align-items: center;
    gap: 2px;
    border-radius: var(--radius-sm);
    border: 1px solid transparent;
  }

  li.selected {
    background: var(--accent-surface);
    border-color: var(--accent-border);
  }

  li.off .label {
    opacity: 0.55;
  }

  li.drop-target {
    box-shadow: inset 0 2px 0 var(--accent);
  }

  li.dragging {
    opacity: 0.6;
  }

  li button {
    border: none;
    background: none;
    padding: 0.3rem;
  }

  .grip {
    cursor: grab;
    color: var(--muted);
    touch-action: none;
  }

  .label {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    text-align: left;
  }

  .name {
    font-size: var(--text-sm);
    font-weight: 600;
  }

  .sub {
    max-width: 100%;
    font-size: var(--text-xs);
    color: var(--fg-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .icon-btn {
    color: var(--muted);
  }
</style>
