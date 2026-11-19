<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type Setting } from '../lib/api'
  import { configVersion } from '../lib/events'
  import Field from '../lib/Field.svelte'
  import Icon from '../lib/Icon.svelte'

  let settings = $state<Setting[]>([])
  let edited = $state<Record<string, string>>({})
  let error = $state('')
  let saved = $state('')
  let loading = $state(true)
  let activeGroup = $state<string>('')

  async function load(preserveEdits = false) {
    loading = true
    try {
      const fresh = await api.settings()
      const prev = new Map(settings.map((s) => [s.key, s.value]))
      const next: Record<string, string> = {}
      for (const s of fresh) {
        const current = edited[s.key]
        const hadUnsavedEdit =
          preserveEdits &&
          current !== undefined &&
          prev.has(s.key) &&
          String(current) !== prev.get(s.key)
        next[s.key] = hadUnsavedEdit ? String(current) : s.value
      }
      settings = fresh
      edited = next
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
    loading = false
  }

  async function reload() {
    error = ''
    saved = ''
    await load(false)
  }

  /** Settings are grouped by the first segment of their dotted key. */
  const groups = $derived.by(() => {
    const out = new Map<string, Setting[]>()
    for (const s of settings) {
      const group = s.key.includes('.') ? s.key.split('.')[0] : 'general'
      const list = out.get(group) ?? []
      list.push(s)
      out.set(group, list)
    }
    return [...out.entries()].sort(([a], [b]) => a.localeCompare(b))
  })

  const filteredGroups = $derived(
    activeGroup ? groups.filter(([g]) => g === activeGroup) : groups,
  )

  const changed = $derived(
    settings
      .filter((s) => String(edited[s.key] ?? '') !== s.value)
      .map((s) => s.key),
  )

  async function save() {
    error = ''
    saved = ''
    const toSave = [...changed]
    try {
      // One request per changed key: the API validates per setting and its
      // error names the accepted values, which is lost if a batch fails.
      for (const key of toSave) {
        await api.setSetting(key, String(edited[key] ?? ''))
      }
      saved = `Saved ${toSave.length} setting${toSave.length === 1 ? '' : 's'}.`
      await load(false)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      // Reload the stored baseline so any writes that did land before the
      // failure are reflected, while keeping the unsaved edit and error.
      await load(true)
      error = msg
    }
  }

  async function reset(key: string) {
    error = ''
    saved = ''
    try {
      await api.unsetSetting(key)
      await load(false)
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  let initialized = false
  $effect(() => {
    void $configVersion
    const preserve = initialized
    initialized = true
    untrack(() => {
      void load(preserve)
    })
  })
</script>

<div class="toolbar sticky-toolbar">
  <div class="toolbar-left">
    <strong>Configuration</strong>
    {#if changed.length > 0}
      <span class="badge accent">{changed.length} unsaved</span>
    {/if}
    {#if saved}
      <span class="badge success">{saved}</span>
    {/if}
  </div>

  <div class="row">
    <button class="btn-primary" onclick={save} disabled={changed.length === 0}>
      <Icon name="check" size={15} />
      <span>Save{changed.length ? ` (${changed.length})` : ''}</span>
    </button>
    <button onclick={reload}>
      <Icon name="reload" size={15} />
      <span>Reload</span>
    </button>
  </div>
</div>

{#if error}
  <p class="banner">{error}</p>
{/if}

{#if loading}
  <div class="empty-state">
    <p class="muted">Loading application settings…</p>
  </div>
{:else}
  {#if groups.length > 1}
    <div class="group-pills" role="tablist" aria-label="Setting categories">
      <button
        type="button"
        class="pill"
        class:active={activeGroup === ''}
        onclick={() => (activeGroup = '')}
      >
        All ({settings.length})
      </button>
      {#each groups as [group, items] (group)}
        <button
          type="button"
          class="pill"
          class:active={activeGroup === group}
          onclick={() => (activeGroup = group)}
        >
          {group} ({items.length})
        </button>
      {/each}
    </div>
  {/if}

  {#each filteredGroups as [group, items] (group)}
    <section class="group-card">
      <div class="group-header">
        <h2>{group}</h2>
        <span class="badge">{items.length}</span>
      </div>
      <div class="group-body">
        {#each items as setting (setting.key)}
          <div class="entry">
            <Field
              spec={setting}
              isDefault={setting.isDefault}
              bind:value={edited[setting.key]}
            />
            {#if !setting.isDefault}
              <button
                class="reset"
                title="Revert to the built-in default"
                onclick={() => reset(setting.key)}
              >
                <Icon name="reload" size={13} />
                <span>Reset</span>
              </button>
            {/if}
          </div>
        {/each}
      </div>
    </section>
  {/each}
{/if}

<style>
  .sticky-toolbar {
    position: sticky;
    top: calc(var(--header-height) + var(--space-2));
    z-index: 10;
    background: var(--bg-glass);
    backdrop-filter: var(--backdrop-blur);
    -webkit-backdrop-filter: var(--backdrop-blur);
  }

  .toolbar-left {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .toolbar-left strong {
    font-size: var(--text-md);
    margin-right: var(--space-1);
  }

  .group-pills {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    margin-bottom: var(--space-4);
  }

  .pill {
    min-height: 1.9rem;
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-pill);
    font-size: var(--text-xs);
    text-transform: capitalize;
    background: var(--surface);
    color: var(--muted);
  }

  .pill.active {
    background: var(--accent-surface);
    border-color: var(--accent-border);
    color: var(--accent);
    font-weight: 600;
  }

  .group-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-xs);
    margin-bottom: var(--space-5);
    overflow: hidden;
  }

  .group-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-3) var(--space-4);
    background: var(--surface-subtle);
    border-bottom: 1px solid var(--border);
  }

  .group-header h2 {
    margin: 0;
    text-transform: uppercase;
    font-size: var(--text-xs);
    letter-spacing: 0.06em;
    color: var(--fg);
  }

  .group-body {
    padding: 0 var(--space-4);
  }

  .entry {
    display: flex;
    align-items: start;
    gap: var(--space-3);
    border-bottom: 1px solid var(--border-subtle);
  }

  .entry:last-child {
    border-bottom: none;
  }

  .entry :global(.field) {
    flex: 1;
    border-bottom: none;
  }

  .reset {
    margin-top: var(--space-3);
    font-size: var(--text-xs);
    min-height: 1.875rem;
    padding: var(--space-1) var(--space-2);
    color: var(--muted);
  }

  .reset:hover:not(:disabled) {
    color: var(--fg);
  }
</style>
