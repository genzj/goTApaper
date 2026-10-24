<script lang="ts">
  import { connect, connected } from './lib/events'
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

  connect()
</script>

<svelte:window on:hashchange={() => (active = tabFromHash())} />

<header>
  <h1>goTApaper</h1>
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
  <span class="status" title={$connected ? 'Connected to the daemon' : 'Reconnecting'}>
    {$connected ? '● live' : '○ offline'}
  </span>
</header>

<main>
  {#if active === 'wallpaper'}
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

  main {
    padding: 1rem;
    max-width: 70rem;
  }
</style>
