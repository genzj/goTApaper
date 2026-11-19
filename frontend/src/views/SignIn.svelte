<script lang="ts">
  import { token, verify } from '../lib/auth'
  import Icon from '../lib/Icon.svelte'

  let { notice = '' }: { notice?: string } = $props()

  let candidate = $state('')
  let error = $state('')
  let checking = $state(false)

  async function submit(event: SubmitEvent) {
    event.preventDefault()
    const value = candidate.trim()
    if (!value) return

    checking = true
    error = ''
    try {
      // Checked before it is kept, so a typo is reported here rather than as
      // every view failing at once.
      if (await verify(value)) {
        token.set(value)
      } else {
        error = 'That token was not accepted.'
      }
    } catch {
      error = 'The daemon is not reachable. Is goTApaper running?'
    }
    checking = false
  }
</script>

<div class="signin">
  <div class="signin-brand">
    <div class="brand-badge">
      <Icon name="logo" size={22} />
    </div>
    <div>
      <h2>Sign in to goTApaper</h2>
      <p class="muted subtitle">Local API authentication</p>
    </div>
  </div>

  {#if notice}
    <p class="banner">{notice}</p>
  {/if}

  <p>
    Opening goTApaper from its tray menu signs in automatically. To sign in
    here instead, paste the token printed by:
  </p>
  <pre>goTApaper token</pre>

  <form onsubmit={submit}>
    <input
      type="password"
      autocomplete="off"
      placeholder="Paste API bearer token…"
      bind:value={candidate}
      disabled={checking}
    />
    <button class="btn-primary" type="submit" disabled={checking || !candidate.trim()}>
      <Icon name="key" size={15} />
      <span>{checking ? 'Checking…' : 'Sign in'}</span>
    </button>
  </form>

  {#if error}
    <p class="error">{error}</p>
  {/if}

  <p class="muted security-note">
    The token is remembered by this browser for this address. Anyone who has
    it can change your configuration and read your provider API keys, so keep
    it to yourself.
  </p>
</div>

<style>
  .signin {
    max-width: 32rem;
    margin: var(--space-8) auto;
    padding: var(--space-6);
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-md);
  }

  .signin-brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
    padding-bottom: var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
  }

  .brand-badge {
    width: 2.75rem;
    height: 2.75rem;
    border-radius: var(--radius-md);
    background: var(--accent);
    color: var(--accent-fg);
    display: grid;
    place-items: center;
    box-shadow: var(--shadow-xs);
    flex-shrink: 0;
  }

  .signin h2 {
    margin: 0;
    font-size: var(--text-xl);
  }

  .subtitle {
    margin: 0.1rem 0 0;
    font-size: var(--text-xs);
  }

  form {
    display: flex;
    gap: var(--space-2);
    margin: var(--space-4) 0;
  }

  form input {
    flex: 1;
    min-width: 0;
  }

  form button {
    padding: var(--space-1) var(--space-4);
  }

  pre {
    background: var(--surface-subtle);
    border: 1px solid var(--border-subtle);
    border-radius: var(--radius-sm);
    padding: var(--space-2) var(--space-3);
    margin: var(--space-3) 0;
    color: var(--fg);
    overflow-x: auto;
  }

  .security-note {
    margin-bottom: 0;
    font-size: var(--text-xs);
    line-height: 1.5;
  }

  @media (max-width: 480px) {
    .signin {
      margin: var(--space-4) auto;
      padding: var(--space-4);
    }

    form {
      flex-direction: column;
    }

    form button {
      width: 100%;
    }
  }
</style>
