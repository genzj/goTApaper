<script lang="ts">
  // Picks a font from the ones installed here, by family then style.
  //
  // The list comes from the server's font index, which only lists fonts the
  // renderer can actually load. A family's sample is drawn in the font itself,
  // fetched on demand through the API: only the families shown are loaded.

  import { authorizedFetch, type FontInfo, type UnsupportedFont } from '../api'
  import Icon from '../Icon.svelte'

  let {
    value = $bindable(),
    fonts,
    unsupported = [],
    id,
    onEdit,
  }: {
    value: string
    fonts: FontInfo[]
    /** Installed fonts the renderer cannot load: never offered, explained. */
    unsupported?: UnsupportedFont[]
    id: string
    onEdit?: () => void
  } = $props()

  let query = $state('')
  let open = $state(false)
  let active = $state(0)

  const families = $derived.by(() => {
    const map = new Map<string, FontInfo[]>()
    for (const f of fonts) {
      const list = map.get(f.family) ?? []
      list.push(f)
      map.set(f.family, list)
    }
    return [...map.entries()].map(([family, styles]) => ({ family, styles }))
  })

  const current = $derived(
    fonts.find((f) => f.file.toLowerCase() === value.toLowerCase()) ?? null,
  )
  const currentUnsupported = $derived(
    current ? null : (unsupported.find((u) => u.file.toLowerCase() === value.toLowerCase()) ?? null),
  )
  // Unsupported fonts matching the search, so typing a family that is not
  // listed says why instead of just finding nothing.
  const unsupportedMatches = $derived(
    query.trim()
      ? unsupported.filter((u) => u.family.toLowerCase().includes(query.trim().toLowerCase()))
      : [],
  )
  let showUnsupported = $state(false)

  const currentStyles = $derived(families.find((f) => f.family === current?.family)?.styles ?? [])

  const shown = $derived(
    families
      .filter((f) => !query.trim() || f.family.toLowerCase().includes(query.trim().toLowerCase()))
      .slice(0, 60),
  )

  // FontFace instances, loaded once per font, named after the font's id.
  const loaded = new Set<string>()
  async function loadFace(f: FontInfo) {
    if (loaded.has(f.id)) return
    loaded.add(f.id)
    try {
      const resp = await authorizedFetch(`/fonts/${f.id}/file`)
      if (!resp.ok) return
      const face = new FontFace(`wm-${f.id}`, await resp.arrayBuffer())
      await face.load()
      document.fonts.add(face)
    } catch {
      // The sample falls back to the interface font; picking still works.
    }
  }

  $effect(() => {
    if (current) void loadFace(current)
  })
  $effect(() => {
    if (open) for (const f of shown.slice(0, 24)) void loadFace(f.styles[0])
  })

  function pick(f: FontInfo) {
    onEdit?.()
    value = f.file
    open = false
    query = ''
  }

  function keydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      open = true
      active = Math.min(active + 1, shown.length - 1)
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      active = Math.max(active - 1, 0)
    } else if (e.key === 'Enter' && open && shown[active]) {
      e.preventDefault()
      pick(shown[active].styles[0])
    } else if (e.key === 'Escape') {
      open = false
    }
  }
</script>

<div class="font-picker">
  <div class="combo">
    <input
      {id}
      type="text"
      role="combobox"
      aria-expanded={open}
      aria-controls={`${id}-list`}
      aria-activedescendant={open && shown[active] ? `${id}-opt-${active}` : undefined}
      placeholder={current ? current.family : value || 'Choose a font'}
      bind:value={query}
      onfocus={() => (open = true)}
      onblur={() => setTimeout(() => (open = false), 150)}
      oninput={() => {
        open = true
        active = 0
      }}
      onkeydown={keydown}
      style:font-family={current ? `wm-${current.id}, var(--font-sans)` : undefined}
    />
    {#if open}
      <ul class="list" id={`${id}-list`} role="listbox">
        {#each shown as f, i (f.family)}
          <li
            id={`${id}-opt-${i}`}
            role="option"
            aria-selected={current?.family === f.family}
            class:active={i === active}
            onpointerdown={(e) => {
              e.preventDefault()
              pick(f.styles[0])
            }}
          >
            <span class="sample" style:font-family={`wm-${f.styles[0].id}, var(--font-sans)`}>{f.family}</span>
            <span class="muted count">{f.styles.length > 1 ? `${f.styles.length} styles` : f.styles[0].style}</span>
          </li>
        {:else}
          {#if unsupportedMatches.length === 0}
            <li class="muted empty">No installed font matches.</li>
          {/if}
        {/each}
        {#each unsupportedMatches as u (u.path)}
          <li class="unsupported" role="option" aria-selected="false" aria-disabled="true" title={u.reason}>
            <span>{u.family} <span class="muted">{u.style}</span></span>
            <span class="warn reason">can't be used: {u.reason}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  {#if currentStyles.length > 1}
    <select
      aria-label="Font style"
      value={current?.file}
      onchange={(e) => {
        const f = currentStyles.find((s) => s.file === (e.currentTarget as HTMLSelectElement).value)
        if (f) pick(f)
      }}
    >
      {#each currentStyles as s (s.id)}
        <option value={s.file}>{s.style}</option>
      {/each}
    </select>
  {/if}
  {#if unsupported.length > 0}
    <!-- Folded by default: a warning icon, expanding to the list on click. -->
    <button
      type="button"
      class="warn-toggle"
      class:open={showUnsupported}
      aria-expanded={showUnsupported}
      aria-controls={`${id}-unsupported`}
      aria-label={`${unsupported.length} installed fonts are not offered; show why`}
      title={`${unsupported.length} installed ${unsupported.length === 1 ? 'font is' : 'fonts are'} not offered — click for details`}
      onclick={() => (showUnsupported = !showUnsupported)}
    >
      <Icon name="alert" size={15} />
      <span class="count">{unsupported.length}</span>
    </button>
  {/if}
</div>
{#if currentUnsupported}
  <p class="warn small">
    <code>{value}</code> is installed, but it {currentUnsupported.reason}, so this layer is not
    drawn. Pick another font.
  </p>
{:else if !current && value}
  <p class="warn small">
    <code>{value}</code> is not installed here, so this layer is not drawn. Pick an installed font.
  </p>
{/if}
{#if unsupported.length > 0 && showUnsupported}
  <div class="unsupported-panel" id={`${id}-unsupported`}>
    <p class="muted small">
      {unsupported.length} installed {unsupported.length === 1 ? 'font is' : 'fonts are'} not offered:
      the renderer cannot read {unsupported.length === 1 ? 'it' : 'them'}.
      Fonts installed since goTApaper started appear after restarting it.
    </p>
    <ul class="unsupported-list">
      {#each unsupported as u (u.path)}
        <li><strong>{u.family}</strong> <span class="muted">{u.style} · {u.file}</span> — {u.reason}</li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .font-picker {
    display: flex;
    gap: var(--space-2);
  }

  .combo {
    position: relative;
    flex: 1;
  }

  .combo input {
    width: 100%;
  }

  .list {
    position: absolute;
    z-index: 20;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    margin: 0;
    padding: var(--space-1);
    list-style: none;
    max-height: 16rem;
    overflow: auto;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
  }

  .list li {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .list li.active,
  .list li[aria-selected='true'] {
    background: var(--accent-surface);
  }

  .sample {
    font-size: var(--text-md);
  }

  .count {
    font-size: var(--text-xs);
    white-space: nowrap;
  }

  .empty {
    cursor: default;
  }

  .unsupported {
    cursor: not-allowed;
    flex-direction: column;
    align-items: flex-start !important;
    opacity: 0.75;
  }

  .reason {
    font-size: var(--text-xs);
  }

  .warn-toggle {
    display: inline-flex;
    align-items: center;
    gap: 0.2rem;
    padding: 0 0.45rem;
    color: var(--warn);
    border-color: var(--warn-border);
    background: var(--warn-surface);
    flex-shrink: 0;
  }

  .warn-toggle.open {
    border-color: var(--warn);
  }

  .warn-toggle .count {
    font-size: var(--text-xs);
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .unsupported-panel {
    margin-top: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--warn-border);
    background: var(--warn-surface);
    border-radius: var(--radius-sm);
  }

  .unsupported-panel .small {
    margin: 0;
  }

  .unsupported-list {
    margin: var(--space-1) 0 0;
    padding-left: var(--space-4);
    max-height: 9rem;
    overflow: auto;
    font-size: var(--text-xs);
  }

  .small {
    font-size: var(--text-xs);
    margin: var(--space-1) 0 0;
  }
</style>
