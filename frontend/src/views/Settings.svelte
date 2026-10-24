<script lang="ts">
  import { api, type Setting } from '../lib/api'
  import { configVersion } from '../lib/events'
  import Field from '../lib/Field.svelte'

  let settings = $state<Setting[]>([])
  let edited = $state<Record<string, string>>({})
  let error = $state('')
  let saved = $state('')
  let loading = $state(true)

  async function load() {
    loading = true
    try {
      settings = await api.settings()
      edited = Object.fromEntries(settings.map((s) => [s.key, s.value]))
      error = ''
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
    loading = false
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
    settings.filter((s) => edited[s.key] !== s.value).map((s) => s.key),
  )

  async function save() {
    error = ''
    saved = ''
    try {
      // One request per changed key: the API validates per setting and its
      // error names the accepted values, which is lost if a batch fails.
      for (const key of changed) {
        await api.setSetting(key, edited[key])
      }
      saved = `Saved ${changed.length} setting${changed.length === 1 ? '' : 's'}.`
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
      // Reload regardless, so the form reflects which writes did land.
      await load()
    }
  }

  async function reset(key: string) {
    error = ''
    try {
      await api.unsetSetting(key)
      await load()
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    }
  }

  $effect(() => {
    void $configVersion
    load()
  })
</script>

<div class="row">
  <button onclick={save} disabled={changed.length === 0}>
    Save{changed.length ? ` (${changed.length})` : ''}
  </button>
  <button onclick={load}>Reload</button>
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
