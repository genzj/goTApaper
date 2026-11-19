<script lang="ts">
  import Icon, { type IconName } from './Icon.svelte'

  export interface NavTab {
    id: string
    label: string
    subtitle: string
    icon: IconName
  }

  let {
    tabs,
    active,
    connected,
    restarting,
    downloading,
    showSignOut = false,
    onSelect,
    onRestart,
    onSignOut,
  }: {
    tabs: readonly NavTab[]
    active: string
    connected: boolean
    restarting: boolean
    downloading: boolean
    showSignOut?: boolean
    onSelect: (id: string) => void
    onRestart: () => void
    onSignOut?: () => void
  } = $props()

  const STORAGE_KEY = 'gotapaper:sidebar-collapsed'
  let collapsed = $state(
    typeof localStorage !== 'undefined' &&
      localStorage.getItem(STORAGE_KEY) === '1',
  )

  function toggleCollapsed() {
    collapsed = !collapsed
    try {
      localStorage.setItem(STORAGE_KEY, collapsed ? '1' : '0')
    } catch {
      // Ignore storage errors in restricted contexts.
    }
  }
</script>

<aside class="sidebar" class:collapsed aria-label="Main navigation">
  <div class="brand">
    {#if !collapsed}
      <div class="brand-mark">
        <Icon name="logo" size={20} />
      </div>
      <div class="brand-text">
        <span class="brand-title">goTApaper</span>
        <span class="brand-tag">Wallpaper Studio</span>
      </div>
    {/if}
    <button
      type="button"
      class="fold-btn"
      onclick={toggleCollapsed}
      aria-expanded={!collapsed}
      aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
      title={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
    >
      <Icon name={collapsed ? 'sidebar-expand' : 'sidebar-collapse'} size={17} />
    </button>
  </div>

  <div class="nav-section">
    {#if !collapsed}
      <span class="nav-heading">Workspace</span>
    {/if}
    <nav class="nav-list">
      {#each tabs as tab (tab.id)}
        <button
          type="button"
          class="nav-item"
          class:active={active === tab.id}
          aria-current={active === tab.id ? 'page' : undefined}
          aria-label={tab.label}
          title={collapsed ? `${tab.label} — ${tab.subtitle}` : undefined}
          onclick={() => onSelect(tab.id)}
        >
          <span class="nav-icon">
            <Icon name={tab.icon} size={18} />
          </span>
          {#if !collapsed}
            <span class="nav-copy">
              <span class="nav-label">{tab.label}</span>
              <span class="nav-sub">{tab.subtitle}</span>
            </span>
          {/if}
        </button>
      {/each}
    </nav>
  </div>

  <div class="sidebar-footer">
    <div
      class="daemon-card"
      class:down={!connected || restarting}
      title={collapsed
        ? restarting
          ? 'Daemon restarting…'
          : connected
            ? 'Daemon online'
            : 'Reconnecting…'
        : undefined}
    >
      <div class="daemon-row">
        <span class="pulse-dot"></span>
        {#if !collapsed}
          <div class="daemon-info">
            <span class="daemon-title">
              {#if restarting}
                Daemon restarting…
              {:else if connected}
                Daemon online
              {:else}
                Reconnecting…
              {/if}
            </span>
            <span class="daemon-meta">
              {#if downloading}
                Fetching wallpaper…
              {:else if connected}
                Event stream active
              {:else}
                Waiting for local API
              {/if}
            </span>
          </div>
        {/if}
      </div>

      <div class="daemon-actions">
        <button
          type="button"
          class="daemon-btn"
          onclick={onRestart}
          disabled={restarting}
          title="Restart the goTApaper daemon"
          aria-label="Restart the goTApaper daemon"
        >
          <Icon name="restart" size={14} />
          {#if !collapsed}
            <span>Restart</span>
          {/if}
        </button>
        {#if showSignOut && onSignOut}
          <button
            type="button"
            class="daemon-btn"
            onclick={onSignOut}
            title="Forget the saved token in this browser"
            aria-label="Sign out"
          >
            <Icon name="signout" size={14} />
            {#if !collapsed}
              <span>Sign out</span>
            {/if}
          </button>
        {/if}
      </div>
    </div>
  </div>
</aside>

<nav class="bottom-nav" aria-label="Mobile navigation">
  {#each tabs as tab (tab.id)}
    <button
      type="button"
      class="bottom-item"
      class:active={active === tab.id}
      aria-current={active === tab.id ? 'page' : undefined}
      onclick={() => onSelect(tab.id)}
    >
      <Icon name={tab.icon} size={19} />
      <span>{tab.label}</span>
    </button>
  {/each}
</nav>

<style>
  .sidebar {
    width: var(--sidebar-width);
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--border);
    background: var(--surface);
    position: sticky;
    top: 0;
    height: 100dvh;
    padding: var(--space-4) var(--space-3);
    z-index: 20;
    transition:
      width var(--duration-normal) var(--ease-out),
      padding var(--duration-normal) var(--ease-out);
  }

  .sidebar.collapsed {
    width: var(--sidebar-collapsed-width);
    padding: var(--space-3) var(--space-2);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-1) var(--space-1) var(--space-4);
    border-bottom: 1px solid var(--border-subtle);
    margin-bottom: var(--space-4);
  }

  .sidebar.collapsed .brand {
    justify-content: center;
    padding: var(--space-1) 0 var(--space-3);
    margin-bottom: var(--space-3);
  }

  .fold-btn {
    margin-left: auto;
    width: 2rem;
    height: 2rem;
    min-height: 2rem;
    padding: 0;
    border-color: transparent;
    background: transparent;
    color: var(--muted);
    box-shadow: none;
    flex-shrink: 0;
  }

  .fold-btn:hover:not(:disabled) {
    background: var(--surface-subtle);
    border-color: var(--border);
    color: var(--fg);
  }

  .sidebar.collapsed .fold-btn {
    margin-left: 0;
    width: 2.35rem;
    height: 2.35rem;
  }

  .brand-mark {
    width: 2.35rem;
    height: 2.35rem;
    border-radius: var(--radius-md);
    background: var(--accent);
    color: var(--accent-fg);
    display: grid;
    place-items: center;
    box-shadow: var(--shadow-xs);
    flex-shrink: 0;
  }

  .brand-text {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .brand-title {
    font-size: var(--text-md);
    font-weight: 700;
    letter-spacing: -0.02em;
    color: var(--fg);
    line-height: 1.2;
  }

  .brand-tag {
    font-size: var(--text-xs);
    color: var(--muted);
    font-weight: 500;
  }

  .nav-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
  }

  .nav-heading {
    font-size: 0.6875rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--muted);
    padding: 0 var(--space-2);
  }

  .nav-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .nav-item {
    width: 100%;
    justify-content: flex-start;
    text-align: left;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    border: 1px solid transparent;
    background: transparent;
    color: var(--fg-secondary);
    box-shadow: none;
    min-height: 2.85rem;
  }

  .sidebar.collapsed .nav-item {
    justify-content: center;
    padding: var(--space-1);
    min-height: 2.6rem;
  }

  .nav-item:hover:not(:disabled) {
    background: var(--surface-subtle);
    border-color: transparent;
    color: var(--fg);
  }

  .nav-item.active {
    background: var(--accent-surface);
    border-color: var(--accent-border);
    color: var(--accent);
  }

  .nav-icon {
    width: 1.85rem;
    height: 1.85rem;
    border-radius: var(--radius-sm);
    display: grid;
    place-items: center;
    background: var(--surface-subtle);
    color: var(--muted);
    flex-shrink: 0;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      color var(--duration-fast) var(--ease-out);
  }

  .nav-item:hover .nav-icon {
    color: var(--fg);
  }

  .nav-item.active .nav-icon {
    background: var(--accent);
    color: var(--accent-fg);
  }

  .nav-copy {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .nav-label {
    font-size: var(--text-base);
    font-weight: 600;
    line-height: 1.25;
  }

  .nav-sub {
    font-size: 0.725rem;
    color: var(--muted);
    font-weight: 400;
    line-height: 1.25;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .nav-item.active .nav-sub {
    color: var(--accent);
    opacity: 0.85;
  }

  .sidebar-footer {
    margin-top: auto;
    padding-top: var(--space-3);
    border-top: 1px solid var(--border-subtle);
  }

  .daemon-card {
    padding: var(--space-3);
    border-radius: var(--radius-md);
    background: var(--surface-subtle);
    border: 1px solid var(--border-subtle);
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .sidebar.collapsed .daemon-card {
    padding: var(--space-2) var(--space-1);
    align-items: center;
    gap: var(--space-2);
  }

  .daemon-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .sidebar.collapsed .daemon-row {
    justify-content: center;
  }

  .pulse-dot {
    width: 0.55rem;
    height: 0.55rem;
    border-radius: var(--radius-pill);
    background: var(--success);
    box-shadow: 0 0 0 3px var(--success-surface);
    flex-shrink: 0;
  }

  .daemon-card.down .pulse-dot {
    background: var(--warn);
    box-shadow: 0 0 0 3px var(--warn-surface);
  }

  .daemon-info {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .daemon-title {
    font-size: var(--text-xs);
    font-weight: 600;
    color: var(--fg);
    line-height: 1.25;
  }

  .daemon-meta {
    font-size: 0.7rem;
    color: var(--muted);
    line-height: 1.3;
  }

  .daemon-actions {
    display: flex;
    gap: var(--space-2);
  }

  .sidebar.collapsed .daemon-actions {
    flex-direction: column;
    width: 100%;
    gap: var(--space-1);
  }

  .daemon-btn {
    flex: 1;
    min-height: 1.85rem;
    padding: var(--space-1) var(--space-2);
    font-size: var(--text-xs);
    background: var(--surface);
  }

  .sidebar.collapsed .daemon-btn {
    padding: var(--space-1);
    justify-content: center;
  }

  .bottom-nav {
    display: none;
  }

  @media (max-width: 820px) {
    .sidebar {
      display: none;
    }

    .bottom-nav {
      display: grid;
      grid-template-columns: repeat(5, 1fr);
      position: fixed;
      bottom: 0;
      left: 0;
      right: 0;
      height: var(--mobile-nav-height);
      background: var(--bg-glass);
      backdrop-filter: var(--backdrop-blur);
      -webkit-backdrop-filter: var(--backdrop-blur);
      border-top: 1px solid var(--border);
      padding: 0.25rem var(--space-2);
      z-index: 30;
    }

    .bottom-item {
      flex-direction: column;
      gap: 0.2rem;
      min-height: 100%;
      padding: 0.2rem;
      border: none;
      background: transparent;
      box-shadow: none;
      color: var(--muted);
      font-size: 0.6875rem;
      font-weight: 500;
      border-radius: var(--radius-sm);
    }

    .bottom-item:hover:not(:disabled) {
      background: var(--surface-subtle);
      color: var(--fg);
    }

    .bottom-item.active {
      color: var(--accent);
      background: var(--accent-surface);
      font-weight: 600;
    }
  }
</style>
