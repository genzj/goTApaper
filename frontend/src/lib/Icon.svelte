<script lang="ts">
  export type IconName =
    | 'logo'
    | 'wallpaper'
    | 'history'
    | 'channels'
    | 'settings'
    | 'logs'
    | 'refresh'
    | 'bolt'
    | 'reload'
    | 'restart'
    | 'signout'
    | 'trash'
    | 'up'
    | 'down'
    | 'sliders'
    | 'check'
    | 'key'
    | 'clock'
    | 'folder'
    | 'link'
    | 'layers'
    | 'sparkles'
    | 'chevron-left'
    | 'chevron-right'
    | 'expand'
    | 'zoom-in'
    | 'zoom-out'
    | 'close'
    | 'fit'
    | 'sidebar-collapse'
    | 'sidebar-expand'
    | 'sun'
    | 'moon'
    | 'monitor'

  let {
    name,
    size = 18,
  }: {
    name: IconName
    size?: number
  } = $props()
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.85"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
  class="icon"
>
  {#if name === 'logo'}
    <rect x="3" y="4" width="18" height="14" rx="2.5" />
    <circle cx="8.5" cy="9.5" r="1.5" />
    <path d="m21 14-4.5-4.5a1.5 1.5 0 0 0-2.12 0L7 17" />
    <path d="M8 21h8" />
  {:else if name === 'wallpaper'}
    <rect x="3" y="4" width="18" height="16" rx="2.5" />
    <circle cx="8.5" cy="9.5" r="1.75" />
    <path d="m21 15-4.5-4.5a1.5 1.5 0 0 0-2.12 0L6 19" />
  {:else if name === 'history'}
    <rect x="3" y="3" width="7.5" height="7.5" rx="1.75" />
    <rect x="13.5" y="3" width="7.5" height="7.5" rx="1.75" />
    <rect x="3" y="13.5" width="7.5" height="7.5" rx="1.75" />
    <rect x="13.5" y="13.5" width="7.5" height="7.5" rx="1.75" />
  {:else if name === 'channels'}
    <path d="M12 3 3 7.5 12 12l9-4.5L12 3Z" />
    <path d="m3 12 9 4.5 9-4.5" />
    <path d="m3 16.5 9 4.5 9-4.5" />
  {:else if name === 'settings'}
    <line x1="4" y1="6" x2="20" y2="6" />
    <line x1="4" y1="12" x2="20" y2="12" />
    <line x1="4" y1="18" x2="20" y2="18" />
    <circle cx="9" cy="6" r="2.2" fill="var(--surface)" />
    <circle cx="15" cy="12" r="2.2" fill="var(--surface)" />
    <circle cx="10" cy="18" r="2.2" fill="var(--surface)" />
  {:else if name === 'logs'}
    <rect x="3" y="4" width="18" height="16" rx="2.5" />
    <path d="m7.5 9.5 2.75 2.5-2.75 2.5" />
    <line x1="12.5" y1="14.5" x2="16.5" y2="14.5" />
  {:else if name === 'refresh' || name === 'reload'}
    <path d="M21 12a9 9 0 0 0-15.36-6.36L3 8" />
    <path d="M3 3v5h5" />
    <path d="M3 12a9 9 0 0 0 15.36 6.36L21 16" />
    <path d="M16 16h5v5" />
  {:else if name === 'bolt'}
    <path d="M13 2 4 14h7l-1 8 9-12h-7l1-8Z" />
  {:else if name === 'restart'}
    <path d="M12 2v4" />
    <path d="M6.34 4.34a8 8 0 1 0 11.32 0" />
  {:else if name === 'signout'}
    <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
    <polyline points="16 17 21 12 16 7" />
    <line x1="21" y1="12" x2="9" y2="12" />
  {:else if name === 'trash'}
    <path d="M3 6h18" />
    <path d="M8 6V4a1.5 1.5 0 0 1 1.5-1.5h5A1.5 1.5 0 0 1 16 4v2" />
    <path d="M19 6v13a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6" />
    <line x1="10" y1="11" x2="10" y2="17" />
    <line x1="14" y1="11" x2="14" y2="17" />
  {:else if name === 'up'}
    <path d="m18 15-6-6-6 6" />
  {:else if name === 'down'}
    <path d="m6 9 6 6 6-6" />
  {:else if name === 'chevron-left'}
    <path d="m15 18-6-6 6-6" />
  {:else if name === 'chevron-right'}
    <path d="m9 18 6-6-6-6" />
  {:else if name === 'sliders'}
    <path d="M12 20v-6M12 8V4M4 20v-2M4 12V4M20 20v-10M20 4v2" />
    <circle cx="12" cy="11" r="2" />
    <circle cx="4" cy="15" r="2" />
    <circle cx="20" cy="8" r="2" />
  {:else if name === 'check'}
    <polyline points="20 6 9 17 4 12" />
  {:else if name === 'key'}
    <circle cx="7.5" cy="15.5" r="4.5" />
    <path d="m10.7 12.3 9.3-9.3" />
    <path d="m16 6 3 3" />
    <path d="m18.5 3.5 2.5 2.5" />
  {:else if name === 'clock'}
    <circle cx="12" cy="12" r="9" />
    <polyline points="12 7 12 12 15.5 14" />
  {:else if name === 'folder'}
    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
  {:else if name === 'link'}
    <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
    <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
  {:else if name === 'layers'}
    <polygon points="12 2 2 7 12 12 22 7 12 2" />
    <polyline points="2 17 12 22 22 17" />
    <polyline points="2 12 12 17 22 12" />
  {:else if name === 'sparkles'}
    <path d="m12 3 1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9L12 3Z" />
    <path d="m19 16 .9 2.1L22 19l-2.1.9L19 22l-.9-2.1L16 19l2.1-.9L19 16Z" />
  {:else if name === 'expand'}
    <polyline points="15 3 21 3 21 9" />
    <polyline points="9 21 3 21 3 15" />
    <line x1="21" y1="3" x2="14" y2="10" />
    <line x1="3" y1="21" x2="10" y2="14" />
  {:else if name === 'zoom-in'}
    <circle cx="11" cy="11" r="7" />
    <line x1="21" y1="21" x2="16.65" y2="16.65" />
    <line x1="11" y1="8" x2="11" y2="14" />
    <line x1="8" y1="11" x2="14" y2="11" />
  {:else if name === 'zoom-out'}
    <circle cx="11" cy="11" r="7" />
    <line x1="21" y1="21" x2="16.65" y2="16.65" />
    <line x1="8" y1="11" x2="14" y2="11" />
  {:else if name === 'close'}
    <line x1="18" y1="6" x2="6" y2="18" />
    <line x1="6" y1="6" x2="18" y2="18" />
  {:else if name === 'fit'}
    <path d="M8 3H5a2 2 0 0 0-2 2v3" />
    <path d="M21 8V5a2 2 0 0 0-2-2h-3" />
    <path d="M3 16v3a2 2 0 0 0 2 2h3" />
    <path d="M16 21h3a2 2 0 0 0 2-2v-3" />
  {:else if name === 'sidebar-collapse'}
    <rect x="3" y="4" width="18" height="16" rx="2.5" />
    <line x1="9" y1="4" x2="9" y2="20" />
    <path d="m15 15-3-3 3-3" />
  {:else if name === 'sidebar-expand'}
    <rect x="3" y="4" width="18" height="16" rx="2.5" />
    <line x1="9" y1="4" x2="9" y2="20" />
    <path d="m13 9 3 3-3 3" />
  {:else if name === 'sun'}
    <circle cx="12" cy="12" r="4" />
    <path d="M12 2v2" />
    <path d="M12 20v2" />
    <path d="m4.93 4.93 1.41 1.41" />
    <path d="m17.66 17.66 1.41 1.41" />
    <path d="M2 12h2" />
    <path d="M20 12h2" />
    <path d="m6.34 17.66-1.41 1.41" />
    <path d="m19.07 4.93-1.41 1.41" />
  {:else if name === 'moon'}
    <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
  {:else if name === 'monitor'}
    <rect x="2" y="3" width="20" height="14" rx="2" />
    <line x1="8" y1="21" x2="16" y2="21" />
    <line x1="12" y1="17" x2="12" y2="21" />
  {/if}
</svg>

<style>
  .icon {
    flex-shrink: 0;
    display: inline-block;
    vertical-align: middle;
  }
</style>
