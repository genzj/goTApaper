<script lang="ts">
  // A colour with transparency: the native picker for the hue, a slider for
  // the alpha, and the hex text for anyone who has an exact value. Writes the
  // rrggbb or rrggbbaa form the renderer reads.

  import { joinColor, splitColor, validColor } from './model'

  let {
    value = $bindable(),
    label,
    optional = false,
    id,
    onEdit,
  }: {
    value: string
    label: string
    /** An optional colour can be empty, meaning "none". */
    optional?: boolean
    id: string
    onEdit?: (key: string) => void
  } = $props()

  const parts = $derived(splitColor(value))
  let text = $state('')
  $effect(() => {
    text = value
  })

  function setRgb(rgb: string) {
    onEdit?.(`${id}-rgb`)
    value = joinColor(rgb, value ? parts.alpha : 1)
  }

  function setAlpha(alpha: number) {
    onEdit?.(`${id}-alpha`)
    value = joinColor(parts.rgb, alpha)
  }

  function commitText() {
    const t = text.trim().replace(/^#/, '')
    if (t === '' && optional) {
      onEdit?.(id)
      value = ''
    } else if (validColor(t)) {
      onEdit?.(id)
      value = t.toLowerCase()
    } else {
      text = value
    }
  }
</script>

<div class="color-input" class:none={!value}>
  {#if optional}
    <input
      type="checkbox"
      checked={Boolean(value)}
      aria-label={`Use a ${label.toLowerCase()}`}
      onchange={(e) => {
        onEdit?.(id)
        value = (e.currentTarget as HTMLInputElement).checked ? '000000aa' : ''
      }}
    />
  {/if}
  <span class="swatch" style:--c={value ? parts.rgb : 'transparent'} style:--a={parts.alpha}>
    <input
      type="color"
      {id}
      value={parts.rgb}
      disabled={!value && optional}
      aria-label={`${label} colour`}
      oninput={(e) => setRgb((e.currentTarget as HTMLInputElement).value)}
    />
  </span>
  <input
    class="hex mono"
    type="text"
    bind:value={text}
    disabled={!value && optional}
    aria-label={`${label} as hexadecimal rrggbb or rrggbbaa`}
    placeholder={optional ? 'none' : 'rrggbb'}
    onchange={commitText}
    onkeydown={(e) => e.key === 'Enter' && commitText()}
  />
  <input
    class="alpha"
    type="range"
    min="0"
    max="1"
    step="0.01"
    value={parts.alpha}
    disabled={!value}
    aria-label={`${label} opacity`}
    oninput={(e) => setAlpha(Number((e.currentTarget as HTMLInputElement).value))}
  />
  <span class="pct muted">{value ? Math.round(parts.alpha * 100) : 0}%</span>
</div>

<style>
  .color-input {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .swatch {
    position: relative;
    width: 1.9rem;
    height: 1.9rem;
    flex-shrink: 0;
    border-radius: var(--radius-sm);
    border: 1px solid var(--border);
    background:
      linear-gradient(color-mix(in srgb, var(--c) calc(var(--a) * 100%), transparent), color-mix(in srgb, var(--c) calc(var(--a) * 100%), transparent)),
      repeating-conic-gradient(#ccc 0 25%, #fff 0 50%) 0 0 / 10px 10px;
    overflow: hidden;
  }

  .swatch input {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    opacity: 0;
    cursor: pointer;
    border: none;
    padding: 0;
  }

  .hex {
    width: 6.5rem;
  }

  .alpha {
    flex: 1;
    min-width: 4rem;
  }

  .pct {
    width: 2.6rem;
    text-align: right;
    font-size: var(--text-xs);
    font-variant-numeric: tabular-nums;
  }

  .none .swatch {
    opacity: 0.5;
  }
</style>
