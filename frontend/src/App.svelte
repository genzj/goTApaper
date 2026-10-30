<script lang="ts">
  import { redeemTicket, signOut, token } from './lib/auth'
  import { connect, connected, disconnect } from './lib/events'
  import SignIn from './views/SignIn.svelte'
  import Wallpaper from './views/Wallpaper.svelte'
  import Settings from './views/Settings.svelte'
  import Channels from './views/Channels.svelte'
  import Logs from './views/Logs.svelte'

  const tabs = [
    { id: 'wallpaper', label: 'Wallpaper' },
    { id: 'settings', label: 'Settings' },
    { id: 'channels', label: 'Channels' },
    { id: 'logs', label: 'Logs & History' },
  ] as const

  type TabID = (typeof tabs)[number]['id']

  // The tab lives in the URL fragment, so reloading the window — which the
  // webview does whenever it is reopened — comes back to the same place.
  function tabFromHash(): TabID {
    const id = window.location.hash.replace(/^#/, '')
    return tabs.some((t) => t.id === id) ? (id as TabID) : 'wallpaper'
  }

  let active = $state<TabID>(tabFromHash())

  function select(id: TabID) {
    active = id
    window.location.hash = id
  }

  // A ticket from the tray is traded before deciding whether to ask for the
  // token, so opening from the tray never flashes the sign-in page.
  let ready = $state(false)
  let notice = $state('')
  redeemTicket()
    .catch((e) => (notice = e instanceof Error ? e.message : String(e)))
    .finally(() => (ready = true))

  // Connect while signed in, and drop the stream on signing out -- including
  // the automatic sign-out when the daemon rejects the token.
  let wasSignedIn = false
  $effect(() => {
    if (!ready) return
    if ($token) {
      wasSignedIn = true
      connect()
      return
    }
    disconnect()
    if (wasSignedIn && !notice) notice = 'The daemon no longer accepts the saved token.'
  })
</script>

<svelte:window on:hashchange={() => (active = tabFromHash())} />

<header>
  <h1>goTApaper</h1>
  {#if $token}
    <nav>
      {#each tabs as tab (tab.id)}
        <button
          class:active={active === tab.id}
          onclick={() => select(tab.id)}
        >
          {tab.label}
        </button>
      {/each}
    </nav>
    <span class="status" class:down={!$connected}>
      {$connected ? 'Daemon connected' : 'Daemon disconnected — reconnecting…'}
    </span>
    <button
      class="signout"
      title="Forget the saved token in this browser"
      onclick={() => {
        notice = 'Signed out.'
        signOut()
      }}>Sign out</button
    >
  {/if}
</header>

<main>
  {#if !ready}
    <p class="muted">Loading…</p>
  {:else if !$token}
    <SignIn {notice} />
  {:else if active === 'wallpaper'}
    <Wallpaper />
  {:else if active === 'settings'}
    <Settings />
  {:else if active === 'channels'}
    <Channels />
  {:else}
    <Logs />
  {/if}
</main>

<style>
  header {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.6rem 1rem;
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    background: var(--bg);
    z-index: 1;
  }

  nav {
    display: flex;
    gap: 0.25rem;
  }

  nav button.active {
    border-color: var(--accent);
    color: var(--accent);
    font-weight: 600;
  }

  .status {
    margin-left: auto;
    color: var(--muted);
    font-size: 0.8rem;
  }

  .status.down {
    color: var(--warn);
  }

  .signout {
    font-size: 0.8rem;
  }

  main {
    padding: 1rem;
    max-width: 70rem;
  }
</style>
