# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

goTApaper is a cross-platform (Windows/macOS/Linux) desktop wallpaper manager written in Go. It downloads images from pluggable sources ("channels") like Bing, National Geographic, Unsplash, and Pexels, optionally crops and watermarks them, and sets them as the desktop wallpaper via OS-specific "setters". It can run one-shot or as a daemon with a system tray icon.

## Common Commands

Toolchain is pinned via `mise.toml` (Go 1.26 — required by Wails v3 — Node 20, `gox` for cross-compilation). `GOPROXY=direct` is set there — the project resolves modules directly from source hosts rather than a proxy.

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
go run . refresh --debug     # download + set wallpaper once
go run . daemon --debug      # tray icon, window and localhost API
go run . daemon --headless --debug   # no tray, no window; for a server or a container
go run . logs -f             # tail the running daemon's log over the event stream
go run . history list        # browse the wallpapers that have been applied
go run . wallpaper current   # what is on the desktop now
go run . config list                 # show every setting, its type and accepted values
go run . config example              # print the documented example configuration
go run . config export               # dump current config as YAML (anchors preserved)
go run . --app-dir /tmp/profile ...   # run against a throwaway config dir

make clean             # removes bin/ and the generated data/example_vfsdata.go
```

Native builds use CGO (`gox -cgo`) because the tray, the webview and the setters link against OS libraries. On Linux this requires `build-essential libgtk-4-dev libwebkitgtk-6.0-dev` — Wails v3 defaults to GTK4 and webkitgtk-6.0. A `gtk3` build tag exists upstream (`gtk+-3.0 webkit2gtk-4.1`) for older distributions, but the GTK4 default is what CI builds. Windows needs the WebView2 runtime at *run* time, not at build time.

CI already builds natively on each OS (`windows-latest`, `ubuntu-24.04`, `macos-latest`), so `gox` acts as an output-naming wrapper rather than a cross-compiler. The 32-bit Linux target is commented out and should stay that way: a 32-bit GTK4 toolchain is not worth standing up. `daemon --headless` is the supported way to run on a machine with no desktop session.

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
- `config/observe.go` is a glob pub/sub `Emitter`, fired by the store **after commit**. The daemon subscribes to `*` to refresh immediately on change. Cross-process changes go through the API: `config set` discovers a running daemon via `<app-dir>/api-endpoint` and routes the write to it (`--local` forces a direct write).

### History (deduplication)

`history/` defines the `Manager` interface; `config/store.HistoryManager` implements it over the database (the adapter lives on the store side to keep the dependency acyclic). One row per channel in `download_history`, as the old `history.json` map was. Channels call `history.Default.Load(name)` / `h.Has(url)` / `h.Mark(url)` to skip re-downloading; `--force` bypasses the check. Note channels key history by their *type* constant, not the instance name, so two instances of one type share an entry.

### Browsable history (`wallpaper_history`)

A **second, append-only** table (migration `0002`), one row per wallpaper actually applied, written by `recordHistory` in `cmd/refresh.go` *after* the setter succeeds. It is deliberately not a widening of `download_history`: merging them would change deduplication from "same as last time?" to "ever seen?", which is a different product decision.

- Each row keeps the source `url`, so `history reapply` / `POST /history/{id}/reapply` can download it again (`cmd/reapply.go`) and run it through the same `applyPicture` as a fresh download. Some providers expire links; that failure is expected and reported.
- A 480px JPEG thumbnail per row under `<app-dir>/thumbs` is what makes an older entry previewable — `wallpaper.file-name` is overwritten every refresh, so only the newest row's file still holds its picture.
- **One retention knob: `history.max-entries`.** No age limit. A thumbnail has no lifetime of its own; it belongs to its row. `history.Sweep` deletes rows over the cap, unlinks their files *after* the transaction commits, then reclaims any thumbnail no row references whose mtime is older than `OrphanGrace` (1h). That grace window matters — a thumbnail is written moments before its row — and the orphan pass is what self-corrects after a crash, a deleted channel or `history.thumbnails` being turned off. Runs at daemon start and after each refresh.
- `--dry-run` reads `WallpaperHistoryBeyond` rather than rolling back a delete, so the preview never takes a write path.

### Localhost REST API

`service/` holds the application operations as plain Go methods on JSON-tagged types, deliberately free of HTTP. `api/` is a thin `net/http` adapter started by the daemon (`--api`, on by default). **The desktop interface also goes through this API rather than binding Go methods** — that is what makes "everything in the UI is doable from the CLI or curl" structural rather than a convention someone has to remember.

- **Binds `127.0.0.1` only** (`api.LoopbackHost`) and requires a bearer token from `<app-dir>/api-token` (mode 0600), compared in constant time. Never make the bind address configurable: the API rewrites configuration and holds the user's provider API keys.
- Secrets are redacted to `service.Redacted` on read; writing that sentinel back is a no-op, so a form can round-trip a channel without ever being told its API key.
- `GET /api/v1/setting-definitions` and `GET /api/v1/channel-types` serve the in-code schemas, so a client validates and renders forms without its own copy of the vocabularies.
- Deleting a channel template that is still inherited returns **409** with the dependent channel names; `POST /api/v1/channel-templates/{name}/inline` is the non-destructive way out.
- `GET /api/v1/events` is one SSE stream carrying `log`, `config-changed`, `refresh-started` and `refresh-finished`. One endpoint, because it works unchanged in the webview, in a browser and under `curl` — `goTApaper logs -f` consumes exactly what the UI does.
- `GET /api/v1/history` is the browsable log; `GET /api/v1/history/latest-per-channel` is the deduplication pointer the CLI's `config history` shows. Two different cardinalities, two paths.

### Browser sessions (`api/session.go`)

A browser cannot set an `Authorization` header on a navigation, so `requireToken` also accepts a `gotapaper_session` cookie carrying the same token. The cookie is set only by `GET /ui/session?ticket=…`, which redeems a **single-use, 60-second ticket** minted in-process by the tray (`Server.SessionURL`) and then redirects to a clean `/ui/`. The token therefore never reaches browser history, a `Referer`, or JavaScript. Mutating requests additionally require an absent or loopback `Origin` (`requireLoopbackOrigin`), as a second line behind `SameSite=Strict`.

**The Wails window loads that same URL** rather than a bundled asset server, so the webview and a browser are one origin with one authentication mechanism. This is why the frontend needs no Wails bindings and `wails3 generate bindings` is not part of the build.

### Desktop interface (`gui/`)

`gui.Run` is the Wails v3 application: tray icon plus a window. It *is* the daemon — the store is single-writer, so a separate GUI process would be a second writer. `cmd/daemon.go` calls it from `runWindowed`, and `runHeadless` runs the same `startRefreshLoop` with no UI for a machine with no desktop session. Wails v3 is beta; the version is pinned and `gui/` is kept thin enough to re-target when its API moves.

### Logging (`logging/`)

A logrus hook feeds every entry into a fixed-size in-memory ring (`log.ring-size`, read by the API and the CLI) and a rotating JSON-lines file (`log.max-size-mb` × `log.max-backups` bounds the disk). **Size caps only, no time-based retention** — `lumberjack.MaxAge` is deliberately unset so one rule decides when a line goes away. Installed from `config.InitLogger`, which re-runs on any `log.*` change.

### Asset embedding

Example configs and fonts in `examples/` are embedded into the binary via `vfsgen` (`generate/assets_generate.go`, build-tagged `dev`). The generated file `data/example_vfsdata.go` is committed but regenerated by `make generate`. `config example` prints the embedded example, which remains the fullest reference for every option since it carries the explanatory comments the database cannot.

## Notes

- The Makefile's `default`/`build` target only builds Linux; for local iteration prefer `go run .` or the specific `build-os-*` target for your platform.
- Version/commit metadata is injected at build time via `-ldflags -X` into `cmd.Version`/`cmd.GitCommit` from `git describe` — don't hardcode these.
- CI (`.github/workflows/build.yml`) builds all platforms, runs `make test`, and drafts a GitHub release on `v*` tags.
