<script lang="ts">
  // The template field: a textarea with completion, contextual help and an
  // inline error marker, all driven by the server's template reference
  // (actor/watermark/fields.go), so nothing here lists a field or function.
  //
  // Completion opens on "{{", on "." or a word inside an action, and on
  // Ctrl+Space anywhere. Help follows the caret: whatever field or function it
  // sits on is explained under the editor, with its value on this picture.

  import type { LayerResult, TemplateField, TemplateFunc, TemplateHelp } from '../api'
  import Icon from '../Icon.svelte'

  let {
    value = $bindable(),
    help,
    result,
    id = 'wm-template',
    onEdit,
  }: {
    value: string
    help: TemplateHelp | null
    result?: LayerResult
    id?: string
    /** Called before every change, so the editor can record an undo step. */
    onEdit?: () => void
  } = $props()

  type Item =
    | { kind: 'field'; name: string; field: TemplateField }
    | { kind: 'func'; name: string; func: TemplateFunc }

  let textarea = $state<HTMLTextAreaElement>()
  let mirror = $state<HTMLDivElement>()
  let open = $state(false)
  let items = $state<Item[]>([])
  let active = $state(0)
  let tokenStart = 0
  let inAction = false
  let popupX = $state(0)
  let popupY = $state(0)
  let caretWord = $state<Item | null>(null)
  let showReference = $state(false)
  let referenceQuery = $state('')

  const allItems = $derived<Item[]>([
    ...(help?.fields ?? []).map((f) => ({ kind: 'field' as const, name: f.name, field: f })),
    ...(help?.funcs ?? []).map((f) => ({ kind: 'func' as const, name: f.name, func: f })),
  ])

  /** Where the caret is relative to the template's actions. */
  function context(text: string, caret: number) {
    const before = text.slice(0, caret)
    const open = before.lastIndexOf('{{')
    const close = before.lastIndexOf('}}')
    const insideAction = open >= 0 && open > close
    const token = /[.\w]*$/.exec(before)?.[0] ?? ''
    return { insideAction, token, start: caret - token.length, justOpened: before.endsWith('{{') }
  }

  function filter(token: string, insideAction: boolean): Item[] {
    const dotted = token.startsWith('.')
    const q = (dotted ? token.slice(1) : token).toLowerCase()
    return allItems
      .filter((it) => {
        if (dotted && it.kind !== 'field') return false
        if (insideAction && !dotted && !q && it.kind === 'field') return true
        return it.name.toLowerCase().startsWith(q) || (q.length > 1 && it.name.toLowerCase().includes(q))
      })
      .slice(0, 12)
  }

  function update(force = false) {
    if (!textarea || !help) return
    const caret = textarea.selectionStart
    const ctx = context(value, caret)
    caretWord = wordAt(value, caret)

    const wants = force || ctx.justOpened || (ctx.insideAction && ctx.token.length > 0)
    if (!wants) {
      open = false
      return
    }
    inAction = ctx.insideAction
    tokenStart = ctx.start
    items = filter(ctx.token, ctx.insideAction)
    active = 0
    open = items.length > 0
    if (open) placePopup(ctx.start)
  }

  /** The field or function the caret is on, for the help line. */
  function wordAt(text: string, caret: number): Item | null {
    const left = /[\w]*$/.exec(text.slice(0, caret))?.[0] ?? ''
    const right = /^[\w]*/.exec(text.slice(caret))?.[0] ?? ''
    const word = left + right
    if (!word) return null
    return allItems.find((it) => it.name === word) ?? null
  }

  // A hidden copy of the textarea, styled identically, tells where the caret
  // is in pixels -- the only reliable way to anchor a popup at it.
  function placePopup(at: number) {
    if (!textarea || !mirror) return
    mirror.textContent = value.slice(0, at)
    const marker = document.createElement('span')
    marker.textContent = '​'
    mirror.appendChild(marker)
    popupX = marker.offsetLeft - textarea.scrollLeft
    popupY = marker.offsetTop + marker.offsetHeight - textarea.scrollTop
  }

  function insertion(it: Item): string {
    if (inAction) {
      if (it.kind === 'field') return '.' + it.name
      return it.name + ' '
    }
    return it.kind === 'field' ? it.field.snippet : it.func.snippet
  }

  function accept(it: Item) {
    if (!textarea) return
    const caret = textarea.selectionStart
    let text = insertion(it)
    // Close an action left open, so accepting "{{" + Title gives "{{.Title}}".
    const rest = value.slice(caret)
    const nextOpen = rest.indexOf('{{')
    const nextClose = rest.indexOf('}}')
    if (inAction && it.kind === 'field' && (nextClose < 0 || (nextOpen >= 0 && nextOpen < nextClose))) {
      text += '}}'
    }
    onEdit?.()
    value = value.slice(0, tokenStart) + text + rest
    open = false
    const pos = tokenStart + text.length
    queueMicrotask(() => {
      textarea?.focus()
      textarea?.setSelectionRange(pos, pos)
      caretWord = wordAt(value, pos - 1)
    })
  }

  function insertSnippet(snippet: string) {
    if (!textarea) return
    const start = textarea.selectionStart
    const end = textarea.selectionEnd
    onEdit?.()
    value = value.slice(0, start) + snippet + value.slice(end)
    const pos = start + snippet.length
    queueMicrotask(() => {
      textarea?.focus()
      textarea?.setSelectionRange(pos, pos)
    })
  }

  function keydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key === ' ') {
      e.preventDefault()
      update(true)
      return
    }
    if (!open) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      active = (active + 1) % items.length
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      active = (active - 1 + items.length) % items.length
    } else if (e.key === 'Enter' || e.key === 'Tab') {
      e.preventDefault()
      accept(items[active])
    } else if (e.key === 'Escape') {
      e.preventDefault()
      e.stopPropagation()
      open = false
    }
  }

  function goToError() {
    const err = result?.error
    if (!textarea || !err?.line) return
    const lines = value.split('\n')
    let pos = 0
    for (let i = 0; i < err.line - 1 && i < lines.length; i++) pos += lines[i].length + 1
    pos += Math.max(0, (err.column ?? 1) - 1)
    textarea.focus()
    textarea.setSelectionRange(pos, Math.min(value.length, pos + 1))
  }

  const errorLine = $derived(result?.error?.stage === 'template' ? (result.error.line ?? 0) : 0)
  const lines = $derived(value.split('\n'))

  const referenceItems = $derived(
    allItems.filter((it) => {
      const q = referenceQuery.trim().toLowerCase()
      if (!q) return true
      const desc = it.kind === 'field' ? it.field.description : it.func.description
      return it.name.toLowerCase().includes(q) || desc.toLowerCase().includes(q)
    }),
  )

  function describe(it: Item): string {
    return it.kind === 'field' ? it.field.description : it.func.description
  }
</script>

<div class="template-editor">
  <div class="input-wrap" class:has-error={errorLine > 0}>
    <!-- Behind the transparent textarea: the same text, with the line the
         renderer complained about marked. -->
    <div class="backdrop" aria-hidden="true">
      {#each lines as line, i (i)}<span class:err-line={i + 1 === errorLine}>{line}</span>{'\n'}{/each}
    </div>
    <textarea
      {id}
      bind:this={textarea}
      bind:value
      rows="3"
      spellcheck="false"
      autocomplete="off"
      role="combobox"
      aria-expanded={open}
      aria-controls={`${id}-completions`}
      aria-autocomplete="list"
      aria-activedescendant={open ? `${id}-opt-${active}` : undefined}
      onbeforeinput={() => onEdit?.()}
      oninput={() => update()}
      onkeydown={keydown}
      onkeyup={(e) => {
        if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) update()
      }}
      onclick={() => {
        open = false
        if (textarea) caretWord = wordAt(value, textarea.selectionStart)
      }}
      onblur={() => setTimeout(() => (open = false), 150)}
      onscroll={(e) => {
        const back = (e.currentTarget as HTMLElement).previousElementSibling as HTMLElement
        back.scrollTop = (e.currentTarget as HTMLElement).scrollTop
      }}
    ></textarea>
    <div class="mirror" bind:this={mirror} aria-hidden="true"></div>

    {#if open}
      <ul
        class="completions"
        id={`${id}-completions`}
        role="listbox"
        style:left={`${Math.max(0, popupX)}px`}
        style:top={`${popupY + 4}px`}
      >
        {#each items as it, i (it.kind + it.name)}
          <li
            id={`${id}-opt-${i}`}
            role="option"
            aria-selected={i === active}
            class:active={i === active}
            onpointerdown={(e) => {
              e.preventDefault()
              accept(it)
            }}
            onpointerenter={() => (active = i)}
          >
            <span class="name">
              <span class="kind" class:func={it.kind === 'func'}>{it.kind === 'field' ? '.' : 'ƒ'}</span>
              {it.name}
              <span class="type">
                {it.kind === 'field' ? it.field.type : it.func.signature}
              </span>
            </span>
            <span class="desc">{describe(it)}</span>
            {#if it.kind === 'field' && it.field.current}
              <span class="current">now: {it.field.current}</span>
            {:else if it.kind === 'func'}
              <span class="current"><code>{it.func.example}</code> → {it.func.result}</span>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>

  <div class="meta-row">
    <span class="muted hint">
      Type <code>{'{{'}</code> or press <kbd>Ctrl</kbd>+<kbd>Space</kbd> for fields and functions.
    </span>
    <button type="button" class="link" onclick={() => (showReference = !showReference)} aria-expanded={showReference}>
      <Icon name="help" size={14} /> Reference
    </button>
  </div>

  {#if result?.error?.stage === 'template'}
    <div class="error-box" role="alert">
      <strong>Template error{result.error.line ? ` — line ${result.error.line}${result.error.column ? `, column ${result.error.column}` : ''}` : ''}:</strong>
      {result.error.message}
      {#if result.error.line}
        <button type="button" class="link" onclick={goToError}>Go to</button>
      {/if}
    </div>
  {:else if result && result.enabled}
    <div class="resolved" aria-live="polite">
      <span class="muted">Renders as</span>
      <pre>{result.text || '(nothing — the template produced empty text)'}</pre>
    </div>
  {/if}

  {#if caretWord}
    <div class="caret-help" aria-live="polite">
      <strong>{caretWord.kind === 'field' ? '.' : ''}{caretWord.name}</strong>
      <span class="muted">
        {caretWord.kind === 'field' ? caretWord.field.type : caretWord.func.signature}
      </span>
      — {describe(caretWord)}
      {#if caretWord.kind === 'field'}
        <span class="muted">
          e.g. <em>{caretWord.field.example}</em>{#if caretWord.field.current}; on this picture: <em>{caretWord.field.current}</em>{/if}
        </span>
      {:else}
        <span class="muted">e.g. <code>{caretWord.func.example}</code> → {caretWord.func.result}</span>
      {/if}
    </div>
  {/if}

  {#if showReference}
    <div class="reference">
      <input type="search" placeholder="Search fields and functions" bind:value={referenceQuery} aria-label="Search the template reference" />
      <table>
        <thead>
          <tr><th>Name</th><th>Meaning</th><th>Example</th><th></th></tr>
        </thead>
        <tbody>
          {#each referenceItems as it (it.kind + it.name)}
            <tr>
              <td class="mono">
                {#if it.kind === 'field'}.{it.name}<div class="muted small">{it.field.type}</div>
                {:else}{it.name}<div class="muted small">{it.func.signature}</div>{/if}
              </td>
              <td>{describe(it)}</td>
              <td>
                {#if it.kind === 'field'}
                  <code>{it.field.snippet}</code>
                  <div class="muted small">
                    {it.field.current ? `now: ${it.field.current}` : `e.g. ${it.field.example}`}
                  </div>
                {:else}
                  <code>{it.func.example}</code>
                  <div class="muted small">→ {it.func.result}</div>
                {/if}
              </td>
              <td>
                <button
                  type="button"
                  onclick={() => insertSnippet(it.kind === 'field' ? it.field.snippet : it.func.snippet)}
                >Insert</button>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<style>
  .template-editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .input-wrap {
    position: relative;
  }

  textarea,
  .backdrop,
  .mirror {
    font-family: var(--font-mono);
    font-size: var(--text-sm);
    line-height: 1.5;
    padding: var(--space-2) var(--space-3);
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    box-sizing: border-box;
    width: 100%;
    border: 1px solid transparent;
  }

  textarea {
    position: relative;
    display: block;
    min-height: 4.5rem;
    resize: vertical;
    background: transparent;
    border-color: var(--border);
    border-radius: var(--radius-sm);
    color: var(--fg);
  }

  .has-error textarea {
    border-color: var(--error-border);
  }

  .backdrop {
    position: absolute;
    inset: 0;
    overflow: hidden;
    color: transparent;
    background: var(--surface);
    border-radius: var(--radius-sm);
    pointer-events: none;
  }

  .err-line {
    background: var(--error-surface);
    text-decoration: underline wavy var(--error);
    text-underline-offset: 3px;
  }

  .mirror {
    position: absolute;
    top: 0;
    left: 0;
    visibility: hidden;
    pointer-events: none;
  }

  .completions {
    position: absolute;
    z-index: 20;
    list-style: none;
    margin: 0;
    padding: var(--space-1);
    width: min(26rem, 90vw);
    max-height: 18rem;
    overflow: auto;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
  }

  .completions li {
    display: grid;
    gap: 0.1rem;
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-sm);
    cursor: pointer;
  }

  .completions li.active {
    background: var(--accent-surface);
  }

  .name {
    font-family: var(--font-mono);
    font-weight: 600;
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }

  .kind {
    display: inline-block;
    width: 1rem;
    text-align: center;
    color: var(--accent);
  }

  .kind.func {
    color: var(--success);
  }

  .type {
    margin-left: auto;
    font-weight: 400;
    font-size: var(--text-xs);
    color: var(--muted);
  }

  .desc {
    font-size: var(--text-xs);
    color: var(--fg-secondary);
  }

  .current {
    font-size: var(--text-xs);
    color: var(--muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .hint {
    font-size: var(--text-xs);
  }

  kbd {
    font-family: var(--font-mono);
    font-size: 0.7rem;
    padding: 0 0.25rem;
    border: 1px solid var(--border);
    border-radius: 3px;
    background: var(--surface-subtle);
  }

  .link {
    border: none;
    background: none;
    padding: 0;
    color: var(--accent);
    font-size: var(--text-xs);
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    cursor: pointer;
  }

  .error-box {
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--error-border);
    background: var(--error-surface);
    color: var(--error);
    border-radius: var(--radius-sm);
  }

  .resolved pre {
    margin: var(--space-1) 0 0;
    padding: var(--space-2);
    background: var(--surface-subtle);
    border-radius: var(--radius-sm);
    white-space: pre-wrap;
    font-size: var(--text-sm);
  }

  .resolved .muted {
    font-size: var(--text-xs);
  }

  .caret-help {
    font-size: var(--text-xs);
    padding: var(--space-2) var(--space-3);
    border-left: 3px solid var(--accent);
    background: var(--accent-surface);
    border-radius: var(--radius-sm);
    line-height: 1.5;
  }

  .reference {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    max-height: 22rem;
    overflow: auto;
  }

  .reference td {
    font-size: var(--text-xs);
    vertical-align: top;
  }

  .small {
    font-size: 0.7rem;
  }
</style>
