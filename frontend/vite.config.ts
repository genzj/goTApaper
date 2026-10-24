import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

const GITKEEP = `This file keeps frontend/dist present on a fresh checkout.

frontend/assets.go embeds this directory, and //go:embed fails on a missing
one -- which would stop the whole module compiling before anyone had a chance
to run "make frontend". The built bundle itself is generated and ignored.
`

/**
 * Re-emits dist/.gitkeep, which emptyOutDir removes on every build.
 *
 * The placeholder is what keeps the Go module compiling on a checkout where
 * the bundle has not been built, so losing it to a build would be a trap for
 * the next person to clone the repository.
 */
function keepDist(): Plugin {
  return {
    name: 'gotapaper-keep-dist',
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: '.gitkeep', source: GITKEEP })
    },
  }
}

export default defineConfig({
  plugins: [svelte(), keepDist()],

  // The bundle is served from /ui/ by the Go API server, so asset URLs have to
  // be built relative to that prefix rather than to the site root.
  base: '/ui/',

  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },

  server: {
    // `npm run dev` binds loopback only. The dev server proxies to a running
    // daemon, so it carries the same authority as the API itself and must not
    // be reachable from off the host.
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,

    // During development the frontend is served by Vite but the API still
    // lives on the daemon's ephemeral port. Set GOTAPAPER_API to the address
    // in <app-dir>/api-endpoint before starting the dev server.
    proxy: {
      '/api': {
        target: `http://${process.env.GOTAPAPER_API ?? '127.0.0.1:8080'}`,
        changeOrigin: false,
      },
    },
  },
})
