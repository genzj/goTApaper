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

# Code generation (MUST run before building — embeds examples/ into data/)
make generate          # runs `go generate` in generate/, producing data/example_vfsdata.go

# Web interface (MUST run before building — frontend/assets.go embeds frontend/dist)
make frontend          # npm install + vite build
make frontend-deps     # npm ci, for a reproducible install
cd frontend && npm run dev   # Vite dev server; set GOTAPAPER_API to the address
                             # in <app-dir>/api-endpoint so /api proxies to the daemon

# Build for a target platform (runs generate + example + i18n as prerequisites)
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
go run . history list        # browse the wallpapers that have been applied
go run . wallpaper last-applied   # what goTApaper set most recently
go run . config list                 # show every setting, its type and accepted values
go run . config example              # print the documented example configuration
go run . config export               # dump current config as YAML (anchors preserved)
go run . --app-dir /tmp/profile ...   # run against a throwaway config dir

make clean             # removes bin/ and the generated data/example_vfsdata.go
```

Native builds use CGO (`gox -cgo`) because the tray, the webview and the setters link against OS libraries. On Linux this requires `build-essential libgtk-4-dev libwebkitgtk-6.0-dev` — Wails v3 defaults to GTK4 and webkitgtk-6.0. A `gtk3` build tag exists upstream (`gtk+-3.0 webkit2gtk-4.1`) for older distributions, but the GTK4 default is what CI builds. Windows needs the WebView2 runtime at *run* time, not at build time.

CI already builds natively on each OS (`windows-latest`, `ubuntu-24.04`, `macos-latest`), so `gox` acts as an output-naming wrapper rather than a cross-compiler. The 32-bit Linux target is commented out and should stay that way: a 32-bit GTK4 toolchain is not worth standing up. `daemon` (headless) is the supported way to run on a machine with no desktop session.

## Architecture

The processing pipeline is: **channel (download) → crop → watermark → write file → setter (apply)**. It is orchestrated in `cmd/refresh.go` (`refresh` → `detectOneChannel`), which is the best single file to read to understand the whole flow.

### Plugin registries (the central pattern)

Both wallpaper sources and OS setters use a shared registry pattern (`util/registry.go`, a `map[string]interface{}`). Implementations self-register in their file's `init()` function, so **adding a new provider means creating a file whose `init()` calls the registry — no central switch statement to edit.**

- **Channels** (`channel/`): implement `channel.Channel` (`Download(channel.Settings) (*bytes.Reader, image.Image, *PictureMeta, error)`) and call `Channels.RegisterWithSchema(name, impl, schema)` in `init()`. The schema declares the options the channel accepts (and which are secret), so the CLI and API can render and validate them without knowing about the channel. The registered `name` is the config `type` value (e.g. `bing-wallpaper`). See `channel/bing.go` as the reference implementation. `PictureMeta` (title/credit/upload time) flows through to watermark templates.
- **Setters** (`actor/setter/`): implement `setter.Setter` (`Set(filename string) error`) and call `Setters.Register(name, impl)` in `init()`. Platform-specific setters use `//go:build` constraints (e.g. `dmssetter.go`, `gnomesetter.go`, `xfcesetter.go` are Linux-only) and typically shell out via the `setWithCommand`/`runCommand` helpers in `setter.go`.

### Configuration (SQLite-backed, with change observation)

- Configuration lives in a SQLite database at `<app-dir>/gotapaper.db`, opened and migrated in `cmd/bootstrap.go` (`loadConfig`). `<app-dir>` resolves from `--app-dir`, then `$GOTAPAPER_APP_DIR`, then an existing legacy `~/.goTApaper`, then the platform location — `%APPDATA%\goTApaper` on Windows (`config/appdir_windows.go`), the dotfile directory elsewhere. The legacy check comes first so an upgrade never strands an existing config. **Use the override in tests and manual checks so you never touch the real user directory.**
- `config/store` defines the `Store` interface and the typed model; `config/store/sqlite` implements it. Migrations are embedded SQL under `config/store/sqlite/migrations`, applied up-only and tracked by `PRAGMA user_version`. The file is backed up before any migration, and a database newer than the binary is refused rather than opened.
- **Valid values live in Go, not in the schema.** `config/settings_schema.go` (`Definitions`) owns each setting's kind, default and enum; `config/enum.go` owns the watermark/crop vocabularies; `channel.OptionSchema` owns per-channel options. The database stores values only. One type, `config.ValueSpec`, backs both settings and channel options (`config.Definition` and `channel.Option` are aliases of it), so there is one place to extend and one validator. This is deliberate: the valid `setter` values vary per OS via build tags (registered via `config.RegisterEnumSource` to avoid an import cycle), and each channel type accepts a different subset of strategies. Adding a value is a code change, never a migration.
- **Setting keys use `namespace.sub-namespace.key-name`** — dots separate namespaces, dashes separate words (`picture.reference-width`, `log.debug`). Renaming a key requires a `LegacyKeys` entry on its `ValueSpec`, or existing YAML stops importing; `config.LookupLegacy` accepts either spelling. Nested YAML mappings flatten to dotted keys on import and nest again on export.
- **Two kinds of version.** `PRAGMA user_version` covers table *structure* and is what the migration runner tracks. `schema_meta` rows cover *meaning*: `config.SettingsSchemaVersion` for the set of setting keys, and `channel_instances.options_version` (from `channel.OptionSchema.Version`) for each channel type's option shape. Either being newer than the binary understands is refused rather than opened. Content versions live in rows, not PRAGMAs, because renaming or re-meaning a value may need Go code rather than SQL.
- Scalar settings are read through `config.GetString/GetBool/GetInt/GetFloat64`, backed by `config.Current()`. `config/store.Provider` implements it with an override layer so CLI flags win for one run **without being persisted**.
- Channel options are layered like a YAML merge key: template options, then the instance's own options, then its promoted `strategy`/`width`/`height` columns (nil means inherit). This resolution is in Go (`sqlite.resolve`), not SQL — `json_patch` treats a null member as *delete*, so a NULL column would erase the inherited value instead of falling back to it.
- `config/store/yamlimport` migrates a legacy `config.yaml` on first run, then renames it `.migrated`. It walks a `yaml.Node` tree rather than unmarshalling, so `&anchor`/`<<:` survive as `channel_templates` instead of being flattened into every channel. `Export` reconstructs them as real anchors, so the round trip is stable.
- `config/observe.go` is a glob pub/sub `Emitter`, fired by the store **after commit**. The daemon subscribes to `*` to schedule a refresh on change (never blocking: a buffered wake channel). Cross-process changes go through the API: `config set` discovers a running daemon via `<app-dir>/api-endpoint` and routes the write to it (`--local` forces a direct write).

### Wallpaper history (`wallpaper_history`) and deduplication

One table, one row per picture applied, written by `recordHistory` in `cmd/refresh.go` *after* the setter succeeds. It is also the deduplication record: `refresh` passes each channel the URL of that channel instance's newest row as `channel.OptionKeyLastAppliedURL` through its `Settings` (next to `OptionKeyForce`), and `channel.alreadyApplied` skips a picture that matches. Channels never touch the store. There used to be a separate `download_history` table and a `history.Manager` global for this; migration `0003` carried its rows over and dropped it.

- **Re-applying updates the entry instead of appending one** (`MarkReapplied`, migration `0004`). `download_time` means *last applied*, so a re-applied entry sorts first, is what dedup compares against, and survives pruning as recently used; `first_applied_time` and `times_applied` keep the rest. Its thumbnail is reused.
- Each row keeps the source `url`, so `history reapply` / `POST /history/{id}/reapply` can download it again (`cmd/reapply.go`, `file://` included) and run it through the same `applyPicture` as a fresh download. Some providers expire links; that failure is expected and reported.
- A 480px JPEG thumbnail per row under `<app-dir>/thumbs` is what makes an older entry previewable — `wallpaper.file-name` is overwritten every refresh, so only the newest row's file still holds its picture.
- **One retention knob: `history.max-entries`.** No age limit. A thumbnail has no lifetime of its own; it belongs to its row. `history.Sweep` deletes rows over the cap, unlinks their files *after* the transaction commits, then reclaims any thumbnail no row references whose mtime is older than `OrphanGrace` (1h). That grace window matters — a thumbnail is written moments before its row — and the orphan pass is what self-corrects after a crash, a deleted channel or `history.thumbnails` being turned off. Runs at daemon start and after each refresh.
- `--dry-run` reads `WallpaperHistoryBeyond` rather than rolling back a delete, so the preview never takes a write path.

### Downloads (`service/downloader.go`)

Every download — timer, config change, tray, API, CLI-via-daemon — goes through one `service.Downloader`, which is `idle` or `downloading`. A user's request starts in the background and answers **202**, or **409** (`ErrBusy`) while another runs: refused, not queued, because a picture appearing long after the user stopped waiting is not what they asked for. A scheduled refresh is never refused; one arriving mid-download runs once afterwards, and several coalesce. Progress is the `download-started` / `download-finished` events and `GET /status`. Never call the pipeline (`refresh`, `reapply`) directly from a running daemon.

### Localhost REST API

**The boundary:** Wails (`gui/`) is for the OS-native experience only — the tray, the window, later global shortcuts. Every application feature, setting and event lives behind the API, for the interface, the CLI and curl alike (and potentially an MCP server later). `service/` holds those operations as plain Go methods on JSON-tagged types, deliberately free of HTTP; `api/` is a thin `net/http` adapter started by `gui` and `daemon` (`--api`, on by default). The interface is just another API client, with no Wails bindings — that is what makes "everything in the UI is doable from the CLI or curl" structural rather than a convention someone has to remember.

- **Binds `127.0.0.1` only** (`api.LoopbackHost`) and requires a bearer token from `<app-dir>/api-token` (mode 0600), compared in constant time. Never make the bind address configurable: the API rewrites configuration and holds the user's provider API keys.
- Secrets are redacted to `service.Redacted` on read; writing that sentinel back is a no-op, so a form can round-trip a channel without ever being told its API key.
- `GET /api/v1/setting-definitions` and `GET /api/v1/channel-types` serve the in-code schemas, so a client validates and renders forms without its own copy of the vocabularies.
- Deleting a channel template that is still inherited returns **409** with the dependent channel names; `POST /api/v1/channel-templates/{name}/inline` is the non-destructive way out.
- `GET /api/v1/events` is one SSE stream carrying `log`, `config-changed`, `download-started` and `download-finished`. One endpoint, because it works unchanged in the webview, in a browser and under `curl` — `goTApaper logs -f` consumes exactly what the UI does.
- Handlers live one file per resource (`api/handlers_*.go`), each registering its own routes.
- `POST /api/v1/daemon/restart` and `/daemon/stop` answer 202 and then act, through `cmd/lifecycle.go`.

### Signing in to the interface (`api/session.go`, `frontend/src/lib/auth.ts`)

Safety first, then simplicity. The interface's static assets under `/ui/` are served **without** a token — the bundle holds no secrets — so the page can load far enough to ask. Every API call it then makes carries `Authorization: Bearer <token>`; cookies are **not** accepted as a credential (a browser sends a cookie on its own, which is what CSRF exploits). The token is kept in `localStorage`; with none stored the page asks, pointing at `goTApaper token`.

The tray signs in without typing by opening `/ui/#ticket=…` — a **single-use, 60-second ticket** the page trades for the token at `POST /api/v1/auth/ticket`, then strips from the address bar. The token itself must never ride in that URL: the browser is launched with it on its command line, readable by any local account via `ps`, and the token file is 0600 precisely to stop that. A ticket rather than a Wails binding, because the same path serves *Open in Browser*, where no binding can reach. The fragment never reaches the server, so tickets never appear in request logs.

With routes now open, `requireLoopbackHost` rejects any request whose `Host` is not loopback (DNS rebinding), and `requireLoopbackOrigin` rejects cross-origin writes. `localStorage` is per origin *including the port*, so with `api.port` at 0 the server first asks for the port it got last time (`<app-dir>/api-port`) — restarts keep the saved token and any open tab working.

### Desktop interface (`gui/`) and the headless daemon

`gui` and `daemon` run the same `startRefreshLoop` (`cmd/daemon.go`); `gui` (`cmd/gui.go`) adds the Wails tray and window. **One `gui` per app dir** (Wails single-instance, ID hashed from the app dir): launching again shows the existing window. A starting `gui` that finds a live `daemon` on its app dir asks it to stop over the API and takes over, so the store keeps one writer. The headless `daemon` is deliberately not limited. Login items register `gui`; `install.MigrateStartUp` rewrites older ones (which started `daemon`) — file only, never the macOS hook, whose `launchctl bootout` would kill the process launchd just started.

- **Restart** re-execs from Wails' `PostShutdown` (headless: after the wait) — on macOS Cocoa `exit()`s after terminating, so `app.Run` never returns and nothing after it runs. Unix uses `syscall.Exec` (same PID; close-on-exec releases the single-instance `flock`); Windows spawns a new process.
- **macOS:** the app is an *accessory* (tray only) until its window opens and *regular* while open, so it appears in Cmd+Tab (`gui/activation_darwin.go`; Wails only sets the policy at launch).
- Wails v3 is beta; the version is pinned and `gui/` is kept thin enough to re-target when its API moves.

### Logging (`logging/`)

A logrus hook feeds every entry into a fixed-size in-memory ring (`log.ring-size`, read by the API and the CLI) and a rotating JSON-lines file (`log.max-size-mb` × `log.max-backups` bounds the disk). **Size caps only, no time-based retention** — `lumberjack.MaxAge` is deliberately unset so one rule decides when a line goes away. Installed from `config.InitLogger`, which re-runs on any `log.*` change.

### Asset embedding

Example configs and fonts in `examples/` are embedded into the binary via `vfsgen` (`generate/assets_generate.go`, build-tagged `dev`). The generated file `data/example_vfsdata.go` is committed but regenerated by `make generate`. `config example` prints the embedded example, which remains the fullest reference for every option since it carries the explanatory comments the database cannot.

## Notes

- The Makefile's `default`/`build` target only builds Linux; for local iteration prefer `go run .` or the specific `build-os-*` target for your platform.
- Version/commit metadata is injected at build time via `-ldflags -X` into `cmd.Version`/`cmd.GitCommit` from `git describe` — don't hardcode these.
- CI (`.github/workflows/build.yml`) builds all platforms, runs `make test`, and drafts a GitHub release on `v*` tags.
