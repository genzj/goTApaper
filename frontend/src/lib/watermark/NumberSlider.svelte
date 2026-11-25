<script lang="ts">
  // A number with a slider for feel and a box for exact values. `scale`
  // shows a fraction as a percentage (0.05 as 5) without changing what is
  // stored.

  let {
    value = $bindable(),
    id,
    label,
    min,
    max,
    step,
    scale = 1,
    unit = '',
    hint = '',
    onEdit,
  }: {
    value: number
    id: string
    label: string
    min: number
    max: number
    step: number
    scale?: number
    unit?: string
    hint?: string
    onEdit?: (key: string) => void
  } = $props()

  const shown = $derived(Math.round(value * scale * 1000) / 1000)

  function set(raw: string) {
    const n = Number(raw)
    if (!Number.isFinite(n)) return
    onEdit?.(id)
    value = Math.min(max, Math.max(min, n / scale))
  }
</script>

<div class="number-slider">
  <label for={id}>{label}</label>
  <input
    type="range"
    min={min * scale}
    max={max * scale}
    step={step * scale}
    value={shown}
    aria-label={label}
    oninput={(e) => set((e.currentTarget as HTMLInputElement).value)}
  />
  <span class="box">
    <input
      {id}
      type="number"
      min={min * scale}
      max={max * scale}
      step={step * scale}
      value={shown}
      onchange={(e) => set((e.currentTarget as HTMLInputElement).value)}
    />
    {#if unit}<span class="unit muted">{unit}</span>{/if}
  </span>
  {#if hint}<span class="hint muted">{hint}</span>{/if}
</div>

<style>
  .number-slider {
    display: grid;
    grid-template-columns: 6.5rem 1fr auto;
    align-items: center;
    gap: var(--space-1) var(--space-2);
  }

  label {
    font-size: var(--text-sm);
    color: var(--fg-secondary);
  }

  .box {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
  }

  .box input {
    width: 4.6rem;
    font-variant-numeric: tabular-nums;
  }

  .unit {
    font-size: var(--text-xs);
    min-width: 1.4rem;
  }

  .hint {
    grid-column: 2 / -1;
    font-size: var(--text-xs);
  }
</style>
