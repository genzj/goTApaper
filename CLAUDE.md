# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

goTApaper is a cross-platform (Windows/macOS/Linux) desktop wallpaper manager written in Go. It downloads images from pluggable sources ("channels") like Bing, National Geographic, Unsplash, and Pexels, optionally crops and watermarks them, and sets them as the desktop wallpaper via OS-specific "setters". It can run one-shot or as a daemon with a system tray icon.

## Common Commands

Toolchain is pinned via `mise.toml` (Go 1.24, `gox` for cross-compilation). `GOPROXY=direct` is set there — the project resolves modules directly from source hosts rather than a proxy.

```bash
# Run all tests
make test              # equivalent to: go test ./...

# Run a single package's tests / a single test
go test ./channel/...
go test ./actor -run TestCrop

# Code generation (MUST run before building — embeds examples/ into data/)
make generate          # runs `go generate` in generate/, producing data/example_vfsdata.go

# Build for a target platform (runs generate + example + i18n as prerequisites)
make build-os-linux-amd64
make build-os-darwin-arm64   # Apple Silicon
make build-os-darwin-amd64   # Intel Mac
make build-os-windows        # GUI + console variants

# Run directly during development (no cross-compile)
go run . refresh --debug     # download + set wallpaper once
go run . daemon --debug      # run tray daemon
go run . config list                 # show every setting, its type and accepted values
go run . config example              # print the documented example configuration
go run . config export               # dump current config as YAML (anchors preserved)
go run . --app-dir /tmp/profile ...   # run against a throwaway config dir

make clean             # removes bin/ and the generated data/example_vfsdata.go
```

Native builds use CGO (`gox -cgo`) because the system tray and setters link against OS libraries. On Linux this requires `build-essential libgtk-3-dev libayatana-appindicator3-dev`.

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

`history/` defines the `Manager` interface; `config/store.HistoryManager` implements it over the database (the adapter lives on the store side to keep the dependency acyclic). One row per channel, as the old `history.json` map was. Channels call `history.Default.Load(name)` / `h.Has(url)` / `h.Mark(url)` to skip re-downloading; `--force` bypasses the check. Note channels key history by their *type* constant, not the instance name, so two instances of one type share an entry.

### Localhost REST API

`service/` holds the application operations as plain Go methods on JSON-tagged types, deliberately free of HTTP so a future GUI can bind them directly. `api/` is a thin `net/http` adapter started by the daemon (`--api`, on by default).

- **Binds `127.0.0.1` only** (`api.LoopbackHost`) and requires a bearer token from `<app-dir>/api-token` (mode 0600), compared in constant time. Never make the bind address configurable: the API rewrites configuration and holds the user's provider API keys.
- Secrets are redacted to `service.Redacted` on read; writing that sentinel back is a no-op, so a form can round-trip a channel without ever being told its API key.
- `GET /api/v1/setting-definitions` and `GET /api/v1/channel-types` serve the in-code schemas, so a client validates and renders forms without its own copy of the vocabularies.
- Deleting a channel template that is still inherited returns **409** with the dependent channel names; `POST /api/v1/channel-templates/{name}/inline` is the non-destructive way out.

### Asset embedding

Example configs and fonts in `examples/` are embedded into the binary via `vfsgen` (`generate/assets_generate.go`, build-tagged `dev`). The generated file `data/example_vfsdata.go` is committed but regenerated by `make generate`. `config example` prints the embedded example, which remains the fullest reference for every option since it carries the explanatory comments the database cannot.

## Notes

- The Makefile's `default`/`build` target only builds Linux; for local iteration prefer `go run .` or the specific `build-os-*` target for your platform.
- Version/commit metadata is injected at build time via `-ldflags -X` into `cmd.Version`/`cmd.GitCommit` from `git describe` — don't hardcode these.
- CI (`.github/workflows/build.yml`) builds all platforms, runs `make test`, and drafts a GitHub release on `v*` tags.
