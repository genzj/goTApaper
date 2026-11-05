# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

goTApaper is a cross-platform (Windows/macOS/Linux) desktop wallpaper manager written in Go. It downloads images from pluggable sources ("channels") like Bing, National Geographic, Unsplash, and Pexels, optionally crops and watermarks them, and sets them as the desktop wallpaper via OS-specific "setters". It can run one-shot, as a desktop application with a tray icon and window (`gui`), or as a headless daemon (`daemon`).

## Common Commands

Toolchain is pinned via `mise.toml` (Go 1.26 — required by Wails v3 — Node 24 LTS, `gox` for cross-compilation). `GOPROXY=direct` is set there — the project resolves modules directly from source hosts rather than a proxy.

```bash
# Run all tests
make test              # equivalent to: go test ./...

# Run a single package's tests / a single test
go test ./channel/...
go test ./actor -run TestCrop

# Web interface (run before building a release — frontend/assets.go embeds frontend/dist;
# plain `go build` still works without it, the binary just reports no web interface)
make frontend          # npm install + vite build
make frontend-deps     # npm ci, for a reproducible install
cd frontend && npm run dev   # Vite dev server; set GOTAPAPER_API to the address
                             # in <app-dir>/api-endpoint so /api proxies to the daemon

# Build for a target platform (builds the frontend and copies examples + i18n as prerequisites;
# the go-build-os-* targets also work on their own, e.g. right after `make clean`)
make build-os-linux-amd64
make build-os-darwin-arm64   # Apple Silicon
make build-os-darwin-amd64   # Intel Mac
make build-os-windows        # GUI + console variants

# Run directly during development (no cross-compile)
go run . refresh --debug     # download + set wallpaper once (via a running daemon if any; --local to force in-process)
go run . gui --debug         # tray icon, window and localhost API -- one per app dir
go run . daemon --debug      # headless: no tray, no window; for a server, container or service manager
go run . token               # print the API token, for signing in to the interface or curl
go run . logs -f             # tail the running daemon's log over the event stream
go run . history list        # browse the wallpapers that have been applied (also: show, reapply, preview, prune, delete)
go run . wallpaper last-applied   # what goTApaper set most recently
go run . config list                 # show every setting, its type and accepted values
go run . config example              # print the documented example configuration
go run . config export               # dump current config as YAML (anchors preserved)
go run . --app-dir /tmp/profile ...   # run against a throwaway config dir -- but see the warning below

make clean             # removes bin/ and the built frontend (keeping frontend/dist/.gitkeep)
```

Native builds use CGO (`gox -cgo`) because the tray, the webview and the setters link against OS libraries. On Linux this requires `build-essential libgtk-4-dev libwebkitgtk-6.0-dev` — Wails v3 defaults to GTK4 and webkitgtk-6.0. A `gtk3` build tag exists upstream (`gtk+-3.0 webkit2gtk-4.1`) for older distributions, but the GTK4 default is what CI builds. Windows needs the WebView2 runtime at *run* time, not at build time.

**Live runs change the real desktop.** `--app-dir` isolates the *configuration*, not the screen: any `refresh`, `gui` or `daemon` run applies a wallpaper through the real setter, and the macOS one sets *every* desktop. There is no no-op setter, so prefer unit tests for anything that only needs to exercise the pipeline, and say so before running one live on someone's machine.

CI already builds natively on each OS (`windows-latest`, `ubuntu-24.04`, `macos-latest`), so `gox` acts as an output-naming wrapper rather than a cross-compiler. The 32-bit Linux target is commented out and should stay that way: a 32-bit GTK4 toolchain is not worth standing up. `daemon` (headless) is the supported way to run on a machine with no desktop session.

## Architecture

The processing pipeline is: **channel (download) → crop → watermark → write file → setter (apply) → history**. It is orchestrated in `cmd/refresh.go` (`refresh` → `detectOneChannel` → `applyPicture` → `recordHistory`), which is the best single file to read to understand the whole flow. Re-applying a history entry (`cmd/reapply.go`) fetches the stored URL instead of asking a channel and joins at `applyPicture`, so both paths crop, watermark and record identically.

### Plugin registries (the central pattern)

Both wallpaper sources and OS setters use a shared registry pattern (`util/registry.go`, a `map[string]interface{}`). Implementations self-register in their file's `init()` function, so **adding a new provider means creating a file whose `init()` calls the registry — no central switch statement to edit.**

- **Channels** (`channel/`): implement `channel.Channel` (`Download(channel.Settings) (*bytes.Reader, image.Image, *PictureMeta, error)`) and call `Channels.RegisterWithSchema(name, impl, schema)` in `init()`. The schema declares the options the channel accepts (and which are secret), so the CLI and API can render and validate them without knowing about the channel. The registered `name` is the config `type` value (e.g. `bing-wallpaper`). See `channel/bing.go` as the reference implementation. `PictureMeta` (title/credit/upload time) flows through to watermark templates. **A channel must set `PictureMeta.URL`** to the picture it downloaded — the history keys on it and re-apply/preview download it again (the Unsplash channel once didn't, and its entries could not be re-applied) — and should call `alreadyApplied(setting, url)` before downloading to skip the picture it applied last time.
- **Setters** (`actor/setter/`): implement `setter.Setter` (`Set(filename string) error`) and call `Setters.Register(name, impl)` in `init()`. Platform-specific setters use `//go:build` constraints (e.g. `dmssetter.go`, `gnomesetter.go`, `xfcesetter.go` are Linux-only) and typically shell out via the `setWithCommand`/`runCommand` helpers in `setter.go`. The macOS setter alternates between two slot files (`wallpaper_osx_0`/`_1`) so the path always differs from the one shown, and prunes the system's wallpaper caches after each set (`wallpapercache_darwin.go`).

### Configuration (SQLite-backed, with change observation)

- Configuration lives in a SQLite database at `<app-dir>/gotapaper.db`, opened and migrated in `cmd/bootstrap.go` (`loadConfig`). `<app-dir>` resolves from `--app-dir`, then `$GOTAPAPER_APP_DIR`, then an existing legacy `~/.goTApaper`, then the platform location — `%APPDATA%\goTApaper` on Windows (`config/appdir_windows.go`), the dotfile directory elsewhere. The legacy check comes first so an upgrade never strands an existing config. **Use the override in tests and manual checks so you never touch the real user directory.** This is enforced: `config.AppDir` panics under a test binary (`testing.Testing()`) unless `SetAppDir` or the env var is set. That guard exists because an API test once swept the real `~/.goTApaper/thumbs` against its own in-memory history and deleted every thumbnail. A package whose tests reach anything under the app dir needs a `TestMain` that sets a temporary one (see `api/api_test.go`).
- `config/store` defines the `Store` interface and the typed model; `config/store/sqlite` implements it. Migrations are embedded SQL under `config/store/sqlite/migrations`, applied up-only and tracked by `PRAGMA user_version`. The file is backed up before any migration, and a database newer than the binary is refused rather than opened.
- **Valid values live in Go, not in the schema.** `config/settings_schema.go` (`Definitions`) owns each setting's kind, default and enum; `config/enum.go` owns the watermark/crop vocabularies; `channel.OptionSchema` owns per-channel options. The database stores values only. One type, `config.ValueSpec`, backs both settings and channel options (`config.Definition` and `channel.Option` are aliases of it), so there is one place to extend and one validator. This is deliberate: the valid `setter` values vary per OS via build tags (contributed one at a time via `config.RegisterEnumValue` to avoid an import cycle), and each channel type accepts a different subset of strategies. Adding a value is a code change, never a migration.
- **Setting keys use `namespace.sub-namespace.key-name`** — dots separate namespaces, dashes separate words (`picture.reference-width`, `log.debug`). Renaming a key requires a `LegacyKeys` entry on its `ValueSpec`, or existing YAML stops importing; `config.LookupLegacy` accepts either spelling. Nested YAML mappings flatten to dotted keys on import and nest again on export.
- **Two kinds of version.** `PRAGMA user_version` covers table *structure* and is what the migration runner tracks. `schema_meta` rows cover *meaning*: `config.SettingsSchemaVersion` for the set of setting keys, and `channel_instances.options_version` (from `channel.OptionSchema.Version`) for each channel type's option shape. Either being newer than the binary understands is refused rather than opened. Content versions live in rows, not PRAGMAs, because renaming or re-meaning a value may need Go code rather than SQL.
- Scalar settings are read through `config.GetString/GetBool/GetInt/GetFloat64`, backed by `config.Current()`. `config/store.Provider` implements it with an override layer so CLI flags win for one run **without being persisted**.
- Channel options are layered like a YAML merge key: template options, then the instance's own options, then its promoted `strategy`/`width`/`height` columns (nil means inherit). This resolution is in Go (`sqlite.resolve`), not SQL — `json_patch` treats a null member as *delete*, so a NULL column would erase the inherited value instead of falling back to it.
- `config/store/yamlimport` migrates a legacy `config.yaml` on first run, then renames it `.migrated`. It walks a `yaml.Node` tree rather than unmarshalling, so `&anchor`/`<<:` survive as `channel_templates` instead of being flattened into every channel. `Export` reconstructs them as real anchors, so the round trip is stable.
- `config/observe.go` is a glob pub/sub `Emitter`, fired by the store **after commit**. The daemon subscribes to `*` to schedule a refresh on change (never blocking: a buffered wake channel). Cross-process changes go through the API: `config set` discovers a running daemon via `<app-dir>/api-endpoint` and routes the write to it (`--local` forces a direct write).

### Wallpaper history (`wallpaper_history`) and deduplication

One table, one row per picture applied, written by `recordHistory` in `cmd/refresh.go` *after* the setter succeeds. It is also the deduplication record: `refresh` passes each channel the URL of that channel instance's newest row as `channel.OptionKeyLastAppliedURL` through its `Settings` (next to `OptionKeyForce`), and `channel.alreadyApplied` skips a picture that matches. Channels never touch the store. There used to be a separate `download_history` table and a `history.Manager` global for this; migration `0003` carried its rows over and dropped it.

- **One entry per picture, keyed by URL** (unique partial index, migration `0005`, which also merged the duplicates recorded before it). A picture applied again — re-applied by hand, or downloaded again by a forced refresh — updates its entry (`MarkReapplied`, migration `0004`) instead of appending one. `download_time` means *last applied*, so such an entry sorts first, is what dedup compares against, and survives pruning as recently used; `first_applied_time` and `times_applied` keep the rest. Its thumbnail is reused, and written only if the file has gone. Nothing compares images, so one photo under two URLs is still two entries. Entries with an empty URL (legacy imports) are exempt from the index.
- Each row keeps the source `url`, so `history reapply` / `POST /history/{id}/reapply` can download it again and run it through the same `applyPicture` as a fresh download. Some providers expire links; that failure is expected and reported. URLs are fetched with `util.FetchPicture`, which also reads the fixed channel's `file://` pictures; convert those with `util.FileURLPath`, never `url.Path` directly, which is wrong on Windows (`/C:/...`).
- A 480px JPEG thumbnail per row under `<app-dir>/thumbs` is what makes an older entry previewable — `wallpaper.file-name` is overwritten every refresh, so only the newest row's file still holds its picture. The API reports `hasThumbnail` (the file is on disk, not merely recorded); an entry without one offers **Load preview** (`POST /history/{id}/preview`, `history preview`), which fetches the picture and saves its thumbnail *without* applying it, so it does not go through the downloader.
- **One retention knob: `history.max-entries`.** No age limit. A thumbnail has no lifetime of its own; it belongs to its row. `history.Sweep` deletes rows over the cap, unlinks their files *after* the transaction commits, then reclaims any thumbnail no row references whose mtime is older than `OrphanGrace` (24h). The grace only ever applies to *unreferenced* files, which normally exist for the seconds between a thumbnail being written and its row; a referenced thumbnail lives exactly as long as its row. The orphan pass is what self-corrects after a crash, a deleted channel or `history.thumbnails` being turned off. Runs at daemon start and after each refresh.
- `--dry-run` reads `WallpaperHistoryBeyond` rather than rolling back a delete, so the preview never takes a write path.

### Downloads (`service/downloader.go`)

Every download — timer, config change, tray, API, CLI-via-daemon — goes through one `service.Downloader`, which is `idle` or `downloading`. A user's request starts in the background and answers **202**, or **409** (`ErrBusy`) while another runs: refused, not queued, because a picture appearing long after the user stopped waiting is not what they asked for. A scheduled refresh is never refused; one arriving mid-download runs once afterwards, and several coalesce. Progress is the `download-started` / `download-finished` events and `GET /status`. Every job records a `Reason` (`api`, `tray`, `timer`, `startup`, `config change: <key>`, `queued: …`) and the start and finish are logged at info, so the log can say *why* a picture changed — a refresh queued behind a re-apply replaces the picture just chosen, which otherwise looks like the re-apply applying the wrong one. Never call the pipeline (`refresh`, `reapply`) directly from a running daemon.

### Localhost REST API

**The boundary:** Wails (`gui/`) is for the OS-native experience only — the tray, the window, later global shortcuts. Every application feature, setting and event lives behind the API, for the interface, the CLI and curl alike (and potentially an MCP server later). `service/` holds those operations as plain Go methods on JSON-tagged types, deliberately free of HTTP; `api/` is a thin `net/http` adapter started by `gui` and `daemon` (`--api`, on by default). The interface is just another API client, with no Wails bindings — that is what makes "everything in the UI is doable from the CLI or curl" structural rather than a convention someone has to remember.

- **Binds `127.0.0.1` only** (`api.LoopbackHost`). Never make the bind address configurable: the API rewrites configuration and holds the user's provider API keys.
- **Bearer token** from `<app-dir>/api-token` (mode 0600), compared in constant time — required unless the request comes *directly* from this machine and `api.require-token` is off (the default). A request carrying a token is always checked; one carrying `Forwarded` / `X-Forwarded-For` / `X-Real-IP` / `X-Forwarded-Host` counts as remote, because a reverse proxy on this machine connects from 127.0.0.1 too (best-effort: it relies on the proxy setting one). Tokenless local access leans entirely on the bind, `requireLoopbackHost` and `requireLoopbackOrigin` — don't weaken those.
- Secrets are redacted to `service.Redacted` on read; writing that sentinel back is a no-op, so a form can round-trip a channel without ever being told its API key.
- `GET /api/v1/setting-definitions` and `GET /api/v1/channel-types` serve the in-code schemas, so a client validates and renders forms without its own copy of the vocabularies.
- Deleting a channel template that is still inherited returns **409** with the dependent channel names; `POST /api/v1/channel-templates/{name}/inline` is the non-destructive way out.
- `GET /api/v1/events` is one SSE stream carrying `log`, `config-changed`, `download-started` and `download-finished`. One endpoint, because it works unchanged in the webview, in a browser and under `curl` — `goTApaper logs -f` consumes exactly what the UI does.
- Handlers live one file per resource (`api/handlers_*.go`), each registering its own routes.
- `POST /api/v1/daemon/restart` and `/daemon/stop` answer 202 and then act, through `cmd/lifecycle.go`.

### Signing in to the interface (`api/session.go`, `frontend/src/lib/auth.ts`)

The interface's static assets under `/ui/` are served **without** a token — the bundle holds no secrets. The page then gets access one of four ways:

1. **goTApaper's own window:** the page posts `wails:runtime:ready` on the webview's native message channel (present only inside the window), Wails emits `WindowRuntimeReady`, and `gui.handOverToken` answers with `ExecJS(signInScript)`, which checks the page's origin before handing over the token. No Wails binding — a page served from 127.0.0.1 can't reliably reach one — and the token never appears in a URL. The window has no Sign out.
2. **A browser opened from the tray:** `/ui/#ticket=…`, a **single-use, 60-second ticket** traded at `POST /api/v1/auth/ticket`, then stripped from the address bar. Never the token itself: the browser's launch command line is readable via `ps` by every local account.
3. **Directly on this machine:** no token needed (unless `api.require-token`); the page probes `/status` without one.
4. **Otherwise** (e.g. behind a reverse proxy): the page asks, pointing at `goTApaper token`, and keeps it in `localStorage`.

API calls send `Authorization: Bearer <token>` when there is one; cookies are **not** a credential (a browser sends them by itself — CSRF). `requireLoopbackHost` rejects non-loopback `Host`s (DNS rebinding) and `requireLoopbackOrigin` cross-origin writes. `localStorage` is per origin *including the port*, so with `api.port` at 0 the server first asks for the port it got last time (`<app-dir>/api-port`).

### Desktop interface (`gui/`) and the headless daemon

`gui` and `daemon` run the same `startRefreshLoop` (`cmd/daemon.go`); `gui` (`cmd/gui.go`) adds the Wails tray and window. **One `gui` per app dir** (Wails single-instance, ID hashed from the app dir): launching again shows the existing window. A starting `gui` that finds a live `daemon` on its app dir asks it to stop over the API and takes over, so the store keeps one writer. The headless `daemon` is deliberately not limited. Login items register `gui`; `install.MigrateStartUp` rewrites older ones (which started `daemon`) — file only, never the macOS hook, whose `launchctl bootout` would kill the process launchd just started.

- **Restart** re-execs from Wails' `PostShutdown` (headless: after the wait) — on macOS Cocoa `exit()`s after terminating, so `app.Run` never returns and nothing after it runs. Unix uses `syscall.Exec` (same PID; close-on-exec releases the single-instance `flock`); Windows spawns a new process.
- **macOS:** the app is an *accessory* (tray only) until its window opens and *regular* while open, so it appears in Cmd+Tab (`gui/activation_darwin.go`; Wails only sets the policy at launch).
- Wails v3 is beta; the version is pinned and `gui/` is kept thin enough to re-target when its API moves.

### Logging (`logging/`)

A logrus hook feeds every entry into a fixed-size in-memory ring (`log.ring-size`, read by the API and the CLI) and a rotating JSON-lines file (`log.max-size-mb` × `log.max-backups` bounds the disk). **Size caps only, no time-based retention** — `lumberjack.MaxAge` is deliberately unset so one rule decides when a line goes away. Installed from `config.InitLogger`, which re-runs on any `log.*` change. The file is readable through the API, so **never log setting or option values** — log the key. A proxy URL can carry credentials and channel options carry API keys; the config observer used to log old and new values and no longer does.

### Asset embedding

The example config and tray icons in `examples/` are embedded with `//go:embed` (`examples/embed.go`) and served through `data.ExampleAssets`, an `http.FileSystem`. There is no generation step, so a fresh clone or `make clean` builds with plain `go build`. `config example` prints the embedded example, which remains the fullest reference for every option since it carries the explanatory comments the database cannot. The web interface is embedded the same way from `frontend/dist` (`frontend/assets.go`); its committed `.gitkeep` is what lets the embed compile before `make frontend` has run.

## Notes

- The Makefile's `default`/`build` target builds Linux only, and includes `linux-i386`, which no longer builds (it would need a 32-bit GTK4 toolchain). Use the specific `build-os-*` target for your platform, or `go run .` for local iteration.
- Version/commit metadata is injected at build time via `-ldflags -X` into `cmd.Version`/`cmd.GitCommit`/`cmd.VersionPrerelease` from `git describe` — don't hardcode these.
- CI (`.github/workflows/build.yml`) builds the frontend and all platforms, runs `make test`, and drafts a GitHub release on `v*` tags. Windows is the job that catches path and port assumptions (`file://` drive letters, no privileged ports) — a test that only passes on Unix is a test bug, not a Windows one.
