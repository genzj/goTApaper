<script lang="ts">
  import {
    ApiError,
    api,
    type ChannelTypeInfo,
    type ChannelView,
    type TemplateView,
    type WatermarkLayer,
  } from '../lib/api'
  import { configVersion } from '../lib/events'
  import Field from '../lib/Field.svelte'

  let channels = $state<ChannelView[]>([])
  let types = $state<ChannelTypeInfo[]>([])
  let templates = $state<TemplateView[]>([])
  let layers = $state<WatermarkLayer[]>([])
  let error = $state('')
  let loading = $state(true)
  let expanded = $state<string | null>(null)
  /** Per-channel option edits, keyed by channel name then option key. */
  let optionEdits = $state<Record<string, Record<string, string>>>({})

  async function load() {
    loading = true
    try {
      ;[channels, types, templates, layers] = await Promise.all([
        api.channels(),
        api.channelTypes(),
        api.channelTemplates(),
        api.watermarkLayers(),
      ])
      optionEdits = Object.fromEntries(
        channels.map((c) => [c.name, optionStrings(c)]),
      )
      error = ''
    } catch (e) {
      error = message(e)
    }
    loading = false
  }

  function message(e: unknown): string {
    return e instanceof Error ? e.message : String(e)
  }

  /** schemaFor returns the option schema declared by a channel's type. */
  function schemaFor(type: string) {
    return types.find((t) => t.name === type)?.schema.options ?? []
  }

  /**
   * optionStrings flattens a channel's effective options to strings for the
   * form. Effective rather than own options, so an inherited template value is
   * visible rather than appearing blank.
   */
  function optionStrings(c: ChannelView): Record<string, string> {
    const out: Record<string, string> = {}
    for (const spec of schemaFor(c.type)) {
      const value = c.effectiveOptions?.[spec.key] ?? c.options?.[spec.key]
      out[spec.key] = value === undefined || value === null ? '' : String(value)
    }
    return out
  }

  async function toggleEnabled(c: ChannelView) {
    error = ''
    try {
      await api.upsertChannel({ ...c, enabled: !c.enabled })
      await load()
    } catch (e) {
      error = message(e)
    }
  }

  async function move(index: number, delta: number) {
    const target = index + delta
    if (target < 0 || target >= channels.length) return

    const names = channels.map((c) => c.name)
    ;[names[index], names[target]] = [names[target], names[index]]

    error = ''
    try {
      await api.reorderChannels(names)
      await load()
    } catch (e) {
      error = message(e)
    }
  }

  async function saveOptions(c: ChannelView) {
    error = ''
    const edits = optionEdits[c.name] ?? {}
    const options: Record<string, unknown> = { ...c.options }

    for (const spec of schemaFor(c.type)) {
      const raw = edits[spec.key]
      if (raw === '' || raw === undefined) continue
      // A secret left as the redaction sentinel is passed through unchanged;
      // the server restores the stored value rather than writing the mask.
      options[spec.key] = coerce(raw, spec.kind)
    }

    try {
      await api.upsertChannel({ ...c, options })
      await load()
    } catch (e) {
      error = message(e)
    }
  }

  function coerce(raw: string, kind: string): unknown {
    switch (kind) {
      case 'int':
        return Number.parseInt(raw, 10)
      case 'float':
        return Number.parseFloat(raw)
      case 'bool':
        return raw === 'true'
      default:
        return raw
    }
  }

  async function removeChannel(name: string) {
    if (!confirm(`Delete the channel "${name}" and its history?`)) return
    error = ''
    try {
      await api.deleteChannel(name)
      await load()
    } catch (e) {
      error = message(e)
    }
  }

  async function removeTemplate(t: TemplateView) {
    error = ''
    try {
      await api.deleteChannelTemplate(t.name)
      await load()
      return
    } catch (e) {
      // The documented 409 names the channels still inheriting from it.
      // Inlining is the non-destructive way out, so offer that rather than
      // just reporting a conflict.
      if (e instanceof ApiError && e.status === 409) {
        const names = (e.usedBy ?? []).join(', ')
        const inline = confirm(
          `${t.name} is still inherited by ${names}.\n\n` +
            `Copy its options into those channels and then delete it? ` +
            `Their effective configuration will not change.`,
        )
        if (inline) {
          try {
            await api.inlineChannelTemplate(t.name)
            await load()
            return
          } catch (inlineErr) {
            error = message(inlineErr)
            return
          }
        }
        return
      }
      error = message(e)
    }
  }

  async function toggleLayer(l: WatermarkLayer) {
    error = ''
    try {
      await api.upsertWatermarkLayer({ ...l, enabled: !l.enabled })
      await load()
    } catch (e) {
      error = message(e)
    }
  }

  $effect(() => {
    void $configVersion
    load()
  })
</script>

{#if error}
  <p class="banner">{error}</p>
{/if}

{#if loading}
  <p class="muted">Loading…</p>
{:else}
  <h2>Channels</h2>
  <p class="muted">
    Attempted in this order on each refresh; the first one to return a picture wins.
  </p>

  {#each channels as channel, index (channel.name)}
    <div class="card">
      <div class="row">
        <input
          type="checkbox"
          checked={channel.enabled}
          title="Include in the refresh rotation"
          onchange={() => toggleEnabled(channel)}
        />
        <strong>{channel.name}</strong>
        <span class="muted">{channel.type}</span>
        {#if channel.templateName}
          <span class="muted">· inherits {channel.templateName}</span>
        {/if}
        <span class="spacer"></span>
        <button onclick={() => move(index, -1)} disabled={index === 0} title="Move up">↑</button>
        <button
          onclick={() => move(index, 1)}
          disabled={index === channels.length - 1}
          title="Move down">↓</button
        >
        <button
          onclick={() => (expanded = expanded === channel.name ? null : channel.name)}
        >
          {expanded === channel.name ? 'Hide' : 'Options'}
        </button>
        <button onclick={() => removeChannel(channel.name)}>Delete</button>
      </div>

      {#if expanded === channel.name}
        {#each schemaFor(channel.type) as spec (spec.key)}
          <Field {spec} bind:value={optionEdits[channel.name][spec.key]} />
        {/each}
        {#if schemaFor(channel.type).length === 0}
          <p class="muted">This channel type declares no options.</p>
        {/if}
        <div class="row actions">
          <button onclick={() => saveOptions(channel)}>Save options</button>
        </div>
      {/if}
    </div>
  {/each}

  <h2>Channel templates</h2>
  <p class="muted">Reusable blocks of options that channels inherit from.</p>

  {#if templates.length === 0}
    <p class="muted">No templates configured.</p>
  {:else}
    <table>
      <thead>
        <tr><th>Name</th><th>Type</th><th>Inherited by</th><th></th></tr>
      </thead>
      <tbody>
        {#each templates as template (template.name)}
          <tr>
            <td>{template.name}</td>
            <td class="muted">{template.type || 'any'}</td>
            <td class="muted">{template.usedBy.join(', ') || '—'}</td>
            <td><button onclick={() => removeTemplate(template)}>Delete</button></td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}

  <h2>Watermark layers</h2>
  <p class="muted">Text overlays drawn on the wallpaper, in this order.</p>

  {#if layers.length === 0}
    <p class="muted">No watermark layers configured.</p>
  {:else}
    <table>
      <thead>
        <tr>
          <th>On</th><th>Position</th><th>Font</th><th>Size</th><th>Template</th>
        </tr>
      </thead>
      <tbody>
        {#each layers as layer (layer.id)}
          <tr>
            <td>
              <input
                type="checkbox"
                checked={layer.enabled}
                onchange={() => toggleLayer(layer)}
              />
            </td>
            <td>{layer.position}</td>
            <td class="muted">{layer.font}</td>
            <td>{layer.point}</td>
            <td class="mono">{layer.template || '—'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  {/if}
{/if}

<style>
  .spacer {
    flex: 1;
  }

  .actions {
    margin-top: 0.5rem;
  }
</style>
