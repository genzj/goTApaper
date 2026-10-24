<script lang="ts">
  // One form control, chosen by the value's declared kind.
  //
  // The vocabulary is never duplicated here: enum members, defaults and
  // secrecy all arrive from the server's own schema (config.Definitions and
  // channel.OptionSchema), so adding a setter or a crop mode in Go needs no
  // change in this file.

  import { REDACTED, type ValueSpec } from './api'

  let {
    spec,
    value = $bindable(),
    isDefault = false,
  }: {
    spec: ValueSpec
    value: string
    isDefault?: boolean
  } = $props()

  const label = $derived(spec.label || spec.key)
  // A secret is shown as the redaction sentinel and round-trips untouched, so
  // a form can be saved without ever having been told the API key.
  const isMasked = $derived(spec.secret === true && value === REDACTED)

  function asBool(v: string): boolean {
    return v === 'true' || v === '1'
  }
</script>

<div class="field">
  <label for={spec.key}>
    {label}
    {#if spec.required}<span class="req" title="Required">*</span>{/if}
    <span class="hint">
      <code>{spec.key}</code>
      {#if spec.description}— {spec.description}{/if}
      {#if isDefault && spec.default}<em>(default: {spec.default})</em>{/if}
    </span>
  </label>

  <div>
    {#if spec.kind === 'bool'}
      <input
        id={spec.key}
        type="checkbox"
        checked={asBool(value)}
        onchange={(e) => (value = e.currentTarget.checked ? 'true' : 'false')}
      />
    {:else if spec.kind === 'enum'}
      <select id={spec.key} bind:value>
        {#each spec.enumValues ?? [] as option (option)}
          <option value={option}>{option}</option>
        {/each}
      </select>
    {:else if spec.kind === 'int'}
      <input id={spec.key} type="number" step="1" bind:value />
    {:else if spec.kind === 'float'}
      <input id={spec.key} type="number" step="any" bind:value />
    {:else if spec.secret}
      <input
        id={spec.key}
        type="password"
        autocomplete="off"
        placeholder={isMasked ? 'unchanged' : ''}
        bind:value
      />
      {#if isMasked}
        <span class="hint">
          Saved. Leave as-is to keep it; type to replace it.
        </span>
      {/if}
    {:else}
      <input id={spec.key} type="text" bind:value />
    {/if}
  </div>
</div>

<style>
  .req {
    color: var(--error);
  }
</style>
