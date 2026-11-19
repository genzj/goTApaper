<script lang="ts">
  import {
    ApiError,
    api,
    type ChannelTypeInfo,
    type ChannelView,
    type TemplateView,
    type WatermarkLayer,
    toInstance,
  } from '../lib/api'
  import { linesToMap, mapToLines } from '../lib/format'
  import { configVersion } from '../lib/events'
  import Field from '../lib/Field.svelte'
  import Icon from '../lib/Icon.svelte'

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
      if (value === undefined || value === null) out[spec.key] = ''
      else if (spec.kind === 'stringMap') out[spec.key] = mapToLines(value)
      else out[spec.key] = String(value)
    }
    return out
  }


  async function toggleEnabled(c: ChannelView) {
    error = ''
    try {
      await api.upsertChannel({ ...toInstance(c), enabled: !c.enabled })
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
      await api.upsertChannel({ ...toInstance(c), options })
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
      case 'stringMap':
        return linesToMap(raw)
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
  <div class="empty-state">
    <p class="muted">Loading channels, templates & watermark layers…</p>
  </div>
{:else}
  <section class="panel-section">
    <div class="section-header">
      <div>
        <h2>Channels</h2>
        <p class="muted">
          Attempted in this order on each refresh; the first one to return a picture wins.
        </p>
      </div>
      <span class="badge">{channels.length} configured</span>
    </div>

    {#each channels as channel, index (channel.name)}
      <div class="card channel-card" class:disabled-card={!channel.enabled}>
        <div class="row channel-header">
          <span class="order-badge">#{index + 1}</span>
          <input
            type="checkbox"
            checked={channel.enabled}
            title="Include in the refresh rotation"
            onchange={() => toggleEnabled(channel)}
          />
          <div class="channel-identity">
            <strong>{channel.name}</strong>
            <div class="channel-tags">
              <span class="badge accent">{channel.type}</span>
              {#if channel.templateName}
                <span class="badge">inherits {channel.templateName}</span>
              {/if}
              {#if !channel.enabled}
                <span class="badge warn">paused</span>
              {/if}
            </div>
          </div>

          <span class="spacer"></span>

          <div class="row channel-controls">
            <button
              class="icon-only"
              onclick={() => move(index, -1)}
              disabled={index === 0}
              title="Move up"
              aria-label="Move up"
            >
              <Icon name="up" size={16} />
            </button>
            <button
              class="icon-only"
              onclick={() => move(index, 1)}
              disabled={index === channels.length - 1}
              title="Move down"
              aria-label="Move down"
            >
              <Icon name="down" size={16} />
            </button>
            <button
              onclick={() => (expanded = expanded === channel.name ? null : channel.name)}
              aria-expanded={expanded === channel.name}
            >
              <Icon name="sliders" size={14} />
              <span>{expanded === channel.name ? 'Hide' : 'Options'}</span>
            </button>
            <button class="btn-danger" onclick={() => removeChannel(channel.name)}>
              <Icon name="trash" size={14} />
              <span>Delete</span>
            </button>
          </div>
        </div>

        {#if expanded === channel.name}
          <div class="options-drawer">
            {#each schemaFor(channel.type) as spec (spec.key)}
              <Field {spec} bind:value={optionEdits[channel.name][spec.key]} />
            {/each}
            {#if schemaFor(channel.type).length === 0}
              <p class="muted no-options">This channel type declares no options.</p>
            {/if}
            <div class="row actions">
              <button class="btn-primary" onclick={() => saveOptions(channel)}>
                <Icon name="check" size={15} />
                <span>Save options</span>
              </button>
            </div>
          </div>
        {/if}
      </div>
    {/each}
  </section>

  <!--
    TODO: edit channel templates here -- create one, change its options, and
    choose which template a channel inherits from. The API already supports
    all of it (POST/PUT /channel-templates, a channel's templateName); only the
    form is missing. Until then: "goTApaper config channel-template set".
  -->
  <section class="panel-section">
    <div class="section-header">
      <div>
        <h2>Channel templates</h2>
        <p class="muted">
          Reusable blocks of options that channels inherit from. Editing them here is
          not available yet; use <code>goTApaper config channel-template set</code>.
        </p>
      </div>
      <span class="badge">{templates.length} templates</span>
    </div>

    {#if templates.length === 0}
      <div class="empty-state compact">
        <p class="muted">No templates configured.</p>
      </div>
    {:else}
      <div class="table-scroll">
        <table>
          <thead>
            <tr><th>Name</th><th>Type</th><th>Inherited by</th><th></th></tr>
          </thead>
          <tbody>
            {#each templates as template (template.name)}
              <tr>
                <td><strong>{template.name}</strong></td>
                <td><span class="badge">{template.type || 'any'}</span></td>
                <td class="muted">{template.usedBy.join(', ') || '—'}</td>
                <td class="cell-right">
                  <button class="btn-danger" onclick={() => removeTemplate(template)}>
                    <Icon name="trash" size={14} />
                    <span>Delete</span>
                  </button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>

  <!--
    TODO: edit watermark layers here -- add, remove and reorder them, and set
    each one's font, size, colour, position and template. The API supports it
    (POST/PUT/DELETE /watermark-layers); only the form is missing, and there
    is no CLI for it either, so for now it is "config export", edit the YAML,
    "config import".
  -->
  <section class="panel-section">
    <div class="section-header">
      <div>
        <h2>Watermark layers</h2>
        <p class="muted">
          Text overlays drawn on the wallpaper, in this order. Only switching them on
          and off is available here yet; to change one, edit the output of
          <code>goTApaper config export</code> and load it back with
          <code>goTApaper config import</code>.
        </p>
      </div>
      <span class="badge">{layers.length} layers</span>
    </div>

    {#if layers.length === 0}
      <div class="empty-state compact">
        <p class="muted">No watermark layers configured.</p>
      </div>
    {:else}
      <div class="table-scroll">
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
                <td><span class="badge">{layer.position}</span></td>
                <td class="muted">{layer.font}</td>
                <td>{layer.point} pt</td>
                <td class="mono">{layer.template || '—'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </section>
{/if}

<style>
  .panel-section {
    margin-bottom: var(--space-8);
  }

  .panel-section:last-child {
    margin-bottom: 0;
  }

  .section-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .section-header h2 {
    margin: 0 0 0.25rem;
  }

  .section-header p {
    margin: 0;
    font-size: var(--text-sm);
    max-width: 44rem;
  }

  .channel-card {
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
  }

  .channel-card.disabled-card {
    background: var(--surface-subtle);
  }

  .channel-header {
    gap: var(--space-3);
  }

  .order-badge {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--muted);
    background: var(--surface-subtle);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: 0.15rem 0.45rem;
  }

  .channel-identity {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .channel-identity strong {
    font-size: var(--text-md);
  }

  .channel-tags {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .icon-only {
    width: 2.125rem;
    padding: 0;
  }

  .options-drawer {
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .no-options {
    margin: var(--space-2) 0;
    font-size: var(--text-sm);
  }

  .spacer {
    flex: 1;
    min-width: var(--space-2);
  }

  .actions {
    margin-top: var(--space-3);
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
    justify-content: flex-end;
  }

  .cell-right {
    text-align: right;
  }

  .empty-state.compact {
    padding: var(--space-5) var(--space-4);
  }

  .empty-state.compact p {
    margin: 0;
  }

  @media (max-width: 640px) {
    .channel-controls {
      width: 100%;
      justify-content: flex-end;
      padding-top: var(--space-2);
      border-top: 1px solid var(--border-subtle);
    }
  }
</style>
