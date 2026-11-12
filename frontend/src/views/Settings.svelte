<script lang="ts">
  import { untrack } from 'svelte'
  import { api, type Setting } from '../lib/api'
  import { configVersion } from '../lib/events'
  import Field from '../lib/Field.svelte'

  let settings = $state<Setting[]>([])
  let edited = $state<Record<string, string>>({})
  let error = $state('')
  let saved = $state('')
  let loading = $state(true)

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

<div class="row">
  <button onclick={save} disabled={changed.length === 0}>
    Save{changed.length ? ` (${changed.length})` : ''}
  </button>
  <button onclick={reload}>Reload</button>
  {#if saved}<span class="muted">{saved}</span>{/if}
</div>

{#if error}
  <p class="banner">{error}</p>
{/if}

{#if loading}
  <p class="muted">Loading…</p>
{:else}
  {#each groups as [group, items] (group)}
    <h2>{group}</h2>
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
            Reset
          </button>
        {/if}
      </div>
    {/each}
  {/each}
{/if}

<style>
  .entry {
    display: flex;
    align-items: start;
    gap: 0.5rem;
  }

  .entry :global(.field) {
    flex: 1;
  }

  .reset {
    margin-top: 0.35rem;
    font-size: 0.8rem;
  }
</style>
