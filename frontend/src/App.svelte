<script lang="ts">
  import { api } from './lib/api'
  import {
    isNative,
    probeLocalAccess,
    redeemTicket,
    requestNativeSignIn,
    signOut,
    signedIn,
    token,
  } from './lib/auth'
  import { connect, connected, disconnect, downloading } from './lib/events'
  import Sidebar from './lib/Sidebar.svelte'
  import TopBar from './lib/TopBar.svelte'
  import SignIn from './views/SignIn.svelte'
  import Wallpaper from './views/Wallpaper.svelte'
  import Settings from './views/Settings.svelte'
  import Channels from './views/Channels.svelte'
  import History from './views/History.svelte'
  import Logs from './views/Logs.svelte'
  import Watermark from './views/Watermark.svelte'

  const tabs = [
    {
      id: 'wallpaper',
      label: 'Wallpaper',
      subtitle: 'Current desktop & refresh',
      icon: 'wallpaper',
    },
    {
      id: 'history',
      label: 'History',
      subtitle: 'Past wallpapers & cleanup',
      icon: 'history',
    },
    {
      id: 'watermark',
      label: 'Watermark',
      subtitle: 'Design overlays with a live preview',
      icon: 'type',
    },
    {
      id: 'channels',
      label: 'Channels',
      subtitle: 'Sources, templates & overlays',
      icon: 'channels',
    },
    {
      id: 'settings',
      label: 'Settings',
      subtitle: 'Application configuration',
      icon: 'settings',
    },
    {
      id: 'logs',
      label: 'Logs',
      subtitle: 'Live stream & log files',
      icon: 'logs',
    },
  ] as const

  type TabID = (typeof tabs)[number]['id']

  // The tab lives in the URL fragment, so reloading the window — which the
  // webview does whenever it is reopened — comes back to the same place.
  function tabFromHash(): TabID {
    const id = window.location.hash.replace(/^#/, '')
    return tabs.some((t) => t.id === id) ? (id as TabID) : 'wallpaper'
  }

  let active = $state<TabID>(tabFromHash())
  const currentTab = $derived(tabs.find((t) => t.id === active) ?? tabs[0])

  function select(id: string) {
    if (!tabs.some((t) => t.id === id)) return
    active = id as TabID
    window.location.hash = id
  }

  // Signed in before deciding whether to ask for the token: natively in
  // goTApaper's own window, or with a ticket in a browser opened from the
  // tray, so neither flashes the sign-in page.
  let ready = $state(false)
  let notice = $state('')
  ;(isNative ? requestNativeSignIn() : redeemTicket())
    .catch((e) => (notice = e instanceof Error ? e.message : String(e)))
    // With no token yet, the daemon may not need one: see probeLocalAccess.
    .then(async () => {
      if (!$token) await probeLocalAccess()
    })
    .finally(() => (ready = true))

  let restarting = $state(false)
  async function restart() {
    if (!confirm('Restart the goTApaper daemon? A download in progress is abandoned.')) return
    restarting = true
    try {
      await api.restartDaemon()
    } catch (e) {
      alert(e instanceof Error ? e.message : String(e))
      restarting = false
    }
  }
  // The restart is over once the stream has dropped and come back.
  let droppedDuringRestart = false
  $effect(() => {
    if (!restarting) return
    if (!$connected) droppedDuringRestart = true
    else if (droppedDuringRestart) {
      droppedDuringRestart = false
      restarting = false
    }
  })

  async function handleSignOut() {
    notice = 'Signed out.'
    signOut()
    // The token is forgotten either way; if the daemon lets this page in
    // without one, it simply carries on.
    await probeLocalAccess()
  }

  // Connect while signed in, and drop the stream on signing out -- including
  // the automatic sign-out when the daemon rejects the token.
  let wasSignedIn = false
  $effect(() => {
    if (!ready) return
    if ($signedIn) {
      wasSignedIn = true
      connect()
      return
    }
    disconnect()
    if (isNative) {
      // The application's own window is never left signed out: a rejected
      // token -- the daemon was reinstalled, say -- is simply fetched again.
      void requestNativeSignIn()
      return
    }
    if (wasSignedIn && !notice) notice = 'The daemon no longer accepts the saved token.'
  })
</script>

<svelte:window on:hashchange={() => (active = tabFromHash())} />

<div class="shell" class:signed-in={$signedIn && ready}>
  {#if ready && $signedIn}
    <Sidebar
      {tabs}
      {active}
      connected={$connected}
      {restarting}
      downloading={$downloading !== null}
      showSignOut={!isNative && Boolean($token)}
      onSelect={select}
      onRestart={restart}
      onSignOut={handleSignOut}
    />
  {/if}

  <div class="workspace">
    {#if ready && $signedIn}
      <TopBar
        title={currentTab.label}
        subtitle={currentTab.subtitle}
        icon={currentTab.icon}
        signedIn={$signedIn}
        connected={$connected}
        {restarting}
        downloading={$downloading !== null}
        showSignOut={!isNative && Boolean($token)}
        onRestart={restart}
        onSignOut={handleSignOut}
      />
    {/if}

    <main>
      {#if !ready}
        <div class="empty-state">
          <p class="muted">Loading workspace…</p>
        </div>
      {:else if !$signedIn && isNative}
        <div class="empty-state">
          <p class="muted">Signing in to local daemon…</p>
        </div>
      {:else if !$signedIn}
        <SignIn {notice} />
      {:else if active === 'wallpaper'}
        <Wallpaper />
      {:else if active === 'settings'}
        <Settings />
      {:else if active === 'watermark'}
        <Watermark />
      {:else if active === 'channels'}
        <Channels />
      {:else if active === 'history'}
        <History />
      {:else}
        <Logs />
      {/if}
    </main>
  </div>
</div>

<style>
  .shell {
    min-height: 100dvh;
    display: flex;
    background: var(--bg);
  }

  .workspace {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
  }

  main {
    flex: 1;
    width: 100%;
    max-width: var(--container-max);
    margin: 0 auto;
    padding: var(--space-6) var(--space-6) var(--space-8);
  }

  @media (max-width: 820px) {
    .shell.signed-in .workspace {
      padding-bottom: calc(var(--mobile-nav-height) + var(--space-4));
    }

    main {
      padding: var(--space-4);
    }
  }
</style>
