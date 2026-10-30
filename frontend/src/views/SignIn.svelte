<script lang="ts">
  import { token, verify } from '../lib/auth'

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
  <h2>Sign in</h2>

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
      placeholder="API token"
      bind:value={candidate}
      disabled={checking}
    />
    <button type="submit" disabled={checking || !candidate.trim()}>
      {checking ? 'Checking…' : 'Sign in'}
    </button>
  </form>

  {#if error}
    <p class="error">{error}</p>
  {/if}

  <p class="muted">
    The token is remembered by this browser for this address. Anyone who has
    it can change your configuration and read your provider API keys, so keep
    it to yourself.
  </p>
</div>

<style>
  .signin {
    max-width: 32rem;
  }

  form {
    display: flex;
    gap: 0.5rem;
  }

  form input {
    flex: 1;
  }

  pre {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.4rem 0.6rem;
  }
</style>
