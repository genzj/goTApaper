<script lang="ts">
  import Icon, { type IconName } from './Icon.svelte'
  import { cycleTheme, theme, themeLabel, type ThemeMode } from './theme'

  let {
    title,
    subtitle,
    icon,
    signedIn = false,
    connected = false,
    restarting = false,
    downloading = false,
    showSignOut = false,
    onRestart,
    onSignOut,
  }: {
    title: string
    subtitle: string
    icon: IconName
    signedIn?: boolean
    connected?: boolean
    restarting?: boolean
    downloading?: boolean
    showSignOut?: boolean
    onRestart?: () => void
    onSignOut?: () => void
  } = $props()

  function themeIcon(mode: ThemeMode): IconName {
    if (mode === 'light') return 'sun'
    if (mode === 'dark') return 'moon'
    return 'monitor'
  }
</script>

<header class="topbar">
  <div class="topbar-title">
    <div class="mobile-brand" aria-hidden="true">
      <Icon name="logo" size={18} />
    </div>
    <div class="title-icon" aria-hidden="true">
      <Icon name={icon} size={18} />
    </div>
    <div class="title-copy">
      <h1>{title}</h1>
      <p class="subtitle">{subtitle}</p>
    </div>
  </div>

  {#if signedIn}
    <div class="topbar-status">
      {#if downloading}
        <span class="badge accent">
          <Icon name="refresh" size={13} />
          <span>Downloading wallpaper…</span>
        </span>
      {/if}

      <span class="status-pill" class:down={!connected || restarting}>
        <span class="dot"></span>
        <span>
          {#if restarting}
            Restarting…
          {:else if connected}
            Connected
          {:else}
            Reconnecting…
          {/if}
        </span>
      </span>

      <button
        type="button"
        class="theme-btn"
        onclick={() => cycleTheme($theme)}
        title={`Color theme: ${themeLabel($theme)} (click to switch)`}
        aria-label={`Color theme: ${themeLabel($theme)}. Click to switch.`}
      >
        <Icon name={themeIcon($theme)} size={15} />
        <span class="theme-label">{themeLabel($theme)}</span>
      </button>

      <div class="mobile-actions">
        {#if onRestart}
          <button
            type="button"
            class="icon-btn"
            onclick={onRestart}
            disabled={restarting}
            title="Restart daemon"
            aria-label="Restart daemon"
          >
            <Icon name="restart" size={16} />
          </button>
        {/if}
        {#if showSignOut && onSignOut}
          <button
            type="button"
            class="icon-btn"
            onclick={onSignOut}
            title="Sign out"
            aria-label="Sign out"
          >
            <Icon name="signout" size={16} />
          </button>
        {/if}
      </div>
    </div>
  {/if}
</header>

<style>
  .topbar {
    position: sticky;
    top: 0;
    z-index: 15;
    min-height: var(--header-height);
    padding: var(--space-3) var(--space-6);
    background: var(--bg-glass);
    backdrop-filter: var(--backdrop-blur);
    -webkit-backdrop-filter: var(--backdrop-blur);
    border-bottom: 1px solid var(--border);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .topbar-title {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
  }

  .mobile-brand {
    display: none;
    width: 2rem;
    height: 2rem;
    border-radius: var(--radius-sm);
    background: var(--accent);
    color: var(--accent-fg);
    place-items: center;
    flex-shrink: 0;
  }

  .title-icon {
    width: 2.15rem;
    height: 2.15rem;
    border-radius: var(--radius-md);
    background: var(--surface);
    border: 1px solid var(--border);
    color: var(--accent);
    display: grid;
    place-items: center;
    box-shadow: var(--shadow-xs);
    flex-shrink: 0;
  }

  .title-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  h1 {
    font-size: var(--text-lg);
    font-weight: 700;
    line-height: 1.25;
    margin: 0;
  }

  .subtitle {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--muted);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .topbar-status {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
  }

  .status-pill {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: 0.25rem var(--space-3);
    border-radius: var(--radius-pill);
    background: var(--success-surface);
    border: 1px solid var(--success-border);
    color: var(--success);
    font-size: var(--text-xs);
    font-weight: 600;
    white-space: nowrap;
  }

  .status-pill .dot {
    width: 0.45rem;
    height: 0.45rem;
    border-radius: var(--radius-pill);
    background: currentColor;
  }

  .status-pill.down {
    background: var(--warn-surface);
    border-color: var(--warn-border);
    color: var(--warn);
  }

  .theme-btn {
    min-height: 2rem;
    padding: 0.2rem var(--space-3);
    font-size: var(--text-xs);
    gap: 0.35rem;
  }

  .mobile-actions {
    display: none;
    align-items: center;
    gap: var(--space-1);
  }

  .icon-btn {
    width: 2rem;
    height: 2rem;
    min-height: 2rem;
    padding: 0;
  }

  @media (max-width: 820px) {
    .topbar {
      padding: var(--space-3) var(--space-4);
    }

    .mobile-brand {
      display: grid;
    }

    .title-icon {
      display: none;
    }

    .mobile-actions {
      display: inline-flex;
    }
  }

  @media (max-width: 540px) {
    .subtitle,
    .theme-label {
      display: none;
    }

    .theme-btn {
      width: 2rem;
      padding: 0;
    }
  }
</style>
