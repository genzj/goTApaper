# Configuration: YAML → SQLite, with a localhost REST API

**Status:** implemented and merged to PR [#59](https://github.com/genzj/goTApaper/pull/59) (`feat/sqlite-config-store`, commit `634d065`).
**Last updated:** 2026-10-23, after two rounds of review.

This began as a forward-looking plan and is now an **as-built record**. Where
the implementation diverged from the original design, the divergence and its
reason are kept — the reasons are the useful part, and several of them are
non-obvious enough to be worth re-reading before extending any of this.

---

## Context

goTApaper's configuration lived entirely in a hand-edited `config.yaml` read
through viper. That blocked the planned desktop GUI (Wails v3 + Svelte) three
ways:

1. **Not round-trippable.** `config.SaveConfig` dumped the whole settings map
   back to YAML, destroying every comment, anchor and merge key. A GUI that
   wrote config would have silently shredded the user's file.
2. **No schema.** viper is a stringly-typed bag. A GUI needs to know fields,
   their types, and which options a given channel accepts.
3. **Polling, not events.** `refresh()` re-read the config file every cycle
   because there was no better change signal.

**Outcome:** a typed configuration model in SQLite owned solely by the Go
process; YAML demoted to a one-time import path; and a service layer exposed
over a loopback-only REST API that serves the CLI and tests today and the GUI
later.

**Explicitly out of scope:** the Wails app, the Svelte frontend, sidecar
packaging. No `wails/v3` dependency was added. The API exists now so the
migration is testable end-to-end and the GUI has something to bind to.

---

## As-built architecture

```
  curl / CLI / (future GUI)
            │  HTTP, 127.0.0.1 only, bearer token
            ▼
      api/            net/http server, route table, JSON errors
      api/client/     client — separate package, no handler deps
      api/wire/       shared: file names, error shape, base path
            │
      service/        plain Go methods on JSON-tagged types, HTTP-free
            │
cmd/* ──────┤
            ▼
     config.Store                     (interface, config/store)
   typed getters/setters + Emitter on commit
   validation against the in-code schemas
            │
   ┌────────┴──────────────┐
   │                       │
store/sqlite        store/yamlimport   (one-shot import + export)
   │
<app-dir>/gotapaper.db
```

**Package layout added:** `config/store`, `config/store/sqlite`,
`config/store/yamlimport`, `service`, `api`, `api/client`, `api/wire`.

### Dependency rules that are load-bearing

Three cycles were hit and resolved during implementation. Re-read this before
adding an import:

- **`config` must not import `config/store`.** The plan said `config.Current()`
  would return the store; that closes `config → store → channel → config`.
  Instead `config` *declares* the narrow `config.Provider` interface it needs,
  and `cmd` installs the store into it at startup via `config.SetCurrent`.
- **`config` must not import `actor/setter`.** `util` reads configuration, and
  `actor/setter` depends on `util`. So the dependency is inverted: each setter
  calls `config.RegisterEnumValue` from its own registration.
- **The history adapter lives in `config/store`, not `history`.** `channel`
  depends on `history`, and the store's tests depend on `channel`, so
  `history → store` would be a cycle. `store.HistoryManager` implements
  `history.Manager` from the store side.

### Dependencies

- **`modernc.org/sqlite` v1.53.0** — pure-Go, so the Windows/macOS/Linux
  cross-compile matrix gains no new cgo dependency (the build already carries
  cgo for the tray and setters). **Pinned deliberately:** latest pulls
  `x/sys` v0.49.0, which requires Go 1.26 and would bump the `go` directive.
- **`gopkg.in/yaml.v3`**, replacing `yaml.v2` — only v3 exposes anchors,
  aliases and the `!!merge` key *before* resolution, which is what preserves
  shared-settings blocks as templates instead of flattening them.
- **No** migration library, **no** HTTP framework, **no** `wails/v3`.
- **Removed:** `spf13/viper`, `yaml.v2`. (`go-viper/mapstructure` stays — it is
  a separate library still used by `util/datastructure.go`.)

---

## The central design decision: valid values live in Go

The database stores **values only**. Kinds, defaults, enums and validation live
in code, next to the feature that consumes them:

- `config/valuespec.go` — `config.ValueSpec`, the one type describing a
  configurable value, plus `ValidateValue`.
- `config/settings_schema.go` — `Definitions()`, the authoritative list of
  global settings.
- `config/enum.go` — watermark position/alignment, crop, strategy
  vocabularies, with membership sets for lookup.
- `channel.OptionSchema` — per-channel-type options, declared in each
  channel's `init()`.

**Why this is load-bearing rather than stylistic**, which is easy to forget:

- The valid `setter` values **vary per OS via build tags**. No table could hold
  a copy that is correct on every platform.
- Each channel type accepts a **different subset of strategies** — bing has
  `largest-no-logo`, unsplash does not. A shared `enum_values` column cannot
  express that.
- Adding a value is a code change, never a migration.

`config.Definition` and `channel.Option` are both **aliases of
`config.ValueSpec`**. They started as two near-identical types and had already
drifted — `channel.Option` carried a `stringMap` kind the config validator knew
nothing about. One type means one place to extend and one definition of a valid
enum.

The schemas are **served over the API** (`GET /setting-definitions`,
`GET /channel-types`) so a GUI never ships its own copy of the vocabularies.

---

## Schema

`config/store/sqlite/migrations/0001_init.sql`, embedded via `embed.FS`.

Tables: `schema_meta`, `settings`, `channel_templates`, `channel_instances`,
`watermark_layers`, `download_history`. One view: `channel_with_template`.

Only **structural** constraints are in SQL — keys, foreign keys, JSON validity,
and numeric invariants such as `probability BETWEEN 0 AND 1`. No
`CHECK (… IN (…))` value lists, per the decision above.

### Setting keys are namespaced

`namespace.sub-namespace.key-name` — dots separate namespaces, dashes separate
words within a name:

```
language
log.debug                log.debug-rendering
network.proxy
picture.crop             picture.reference-width   picture.reference-height
wallpaper.setter         wallpaper.file-name
daemon.interval          history.file              api.port
```

Every renamed spec carries `LegacyKeys`, and the importer resolves through
`config.LookupLegacy`, so an existing `config.yaml` with the old flat names
still imports. **Renaming a key without adding a `LegacyKeys` entry silently
breaks import for existing users.**

### Channel templates replace YAML anchors

`channel_templates` + a nullable `channel_instances.template_id`. A shared API
key is configured once instead of copied into every instance.

- **`ON DELETE RESTRICT`**, not `SET NULL`: clearing the reference would
  silently strip a channel of its inherited key and dimensions, leaving a
  config that looks complete and fails only at download time.
  `InlineChannelTemplate` is the non-destructive way out — it copies the
  options down into each dependant, then deletes the template.
- Options layer like a merge key: **template → instance options → promoted
  `strategy`/`width`/`height` columns**, where `nil` means inherit.

### Two kinds of version — do not conflate them

- **`PRAGMA user_version`** — table *structure*. Drives the migration runner.
- **`schema_meta` rows + `channel_instances.options_version`** — *meaning*:
  the set of setting keys (`config.SettingsSchemaVersion`), and each channel
  type's option shape (`channel.OptionSchema.Version`).

Either being newer than the binary understands is **refused**
(`store.ErrSchemaTooNew`) rather than opened. An older build would otherwise
read a renamed key as absent, fall back to a default, and run a configuration
the user never chose.

Content versions live in **rows, not PRAGMAs**, because a content migration may
not be expressible in SQL: renaming an option or splitting one value into two
needs Go code, so the version must be readable and writable from Go at any
time, not only during a migration. The current implementation stops at
*identification* — there is deliberately no speculative migration framework.

### Migrations are up-only

No down-migrations. This is a single-user desktop app where "downgrade" means
someone installed an older build, and a hand-written down-migration that drops
a column is a data-loss bug waiting to happen. Two cheaper mechanisms:

- **Pre-migration backup** to `<db>.pre-v<N>.bak` (most recent two kept), so
  downgrading is "restore the backup".
- **Refuse forward-dated databases.**

Versions must be **unique and consecutive from 1**, enforced by
`TestMigrationsAreWellFormed` in CI rather than only at runtime. Two migrations
at one version means only one ever applies and which one depends on filename
order; a gap means a later fill-in is skipped outright on a database already
past it.

---

## Application directory

Resolved in order:

1. `--app-dir` flag
2. `$GOTAPAPER_APP_DIR`
3. **an existing `~/.goTApaper`** — checked before the platform default, so an
   upgrade never strands a configuration where the new build stops looking
4. platform location: `%APPDATA%\goTApaper` on Windows
   (`config/appdir_windows.go`), the dotfile directory elsewhere
   (`config/appdir_unix.go`)

Linux was deliberately **not** moved to `XDG_CONFIG_HOME` — that is a migration
for existing users and was out of scope here.

The override exists so **no test or manual check ever writes to the real
`~/.goTApaper`**.

---

## Localhost REST API

`service/` holds the operations as plain Go methods on JSON-tagged types,
deliberately HTTP-free so the GUI can bind them directly rather than going
through a socket. `api/` is a thin `net/http` adapter started by the daemon,
filling in the long-standing `//api.StartApiServer()` stub.

Security posture — the API rewrites configuration, can be driven to issue
outbound requests, and holds the user's provider API keys:

- **Binds `127.0.0.1` only** (`api.LoopbackHost`). **Never make this
  configurable.**
- **Bearer token** at `<app-dir>/api-token`, mode `0600`, compared with
  `crypto/subtle.ConstantTimeCompare`. Loopback alone is not an authorisation
  boundary on a shared machine. `GET /healthz` is the only unauthenticated
  route. Rejected requests log at **warning** (not debug) — it is the only
  signal a user gets that something local is probing the interface — and never
  log the presented credential.
- **Secrets redacted on read** to `service.Redacted`; writing the sentinel back
  is a no-op, so a form can round-trip a channel without ever being told its
  API key.
- **Every error is JSON**, including unrouted paths, unsupported methods and
  panics.
- **Concurrent daemon starts leave the last writer's token on disk.** Accepted,
  not defended against — see "Token file" below.

`config set` discovers a running daemon via `<app-dir>/api-endpoint` and routes
the write to it, so the daemon reacts immediately and SQLite keeps a single
writer. `--local` forces a direct write.

---

## Decisions worth not re-litigating

### Option resolution is in Go, not SQL

The original plan specified a `channel_effective` view using `json_patch`. **It
is wrong.** A NULL instance column emits `{"strategy":null}`, and `json_patch`
treats a null member as *delete* — so it would erase the template's inherited
value instead of falling back to it, the exact opposite of what NULL means
here.

The `channel_with_template` view resolves the template *reference* to a name
(which every read wants) but deliberately **not** the option values. Guarded by
`TestResolveInheritsWhenPromotedColumnIsNull`. There is a comment on the view
itself so nobody extends it that way later.

### Token file: three attempts, ending where it started

1. `O_EXCL` — looked right, passed locally.
2. CI caught a race: `O_EXCL` creates the file **empty** and the content lands
   a moment later, so a loser reading between those points saw nothing — and
   the empty-file cleanup could delete the file the winner was about to fill.
   Replaced with write-temp-then-`os.Link`, atomic in *content*.
3. **Reverted to plain `os.WriteFile`** on review: `os.Link` is unavailable on
   some filesystems, so it needed a `Rename` fallback that gave up the very
   guarantee it existed for. Last-writer-wins is accepted; a superseded client
   gets a 401 and the CLI falls back to a direct database write.

An explicit `os.Chmod` follows the write, because `WriteFile` only applies its
mode when *creating* and would leave a pre-existing file's permissions alone.

### `util.RegistryMap` stays shared

A review comment suggested folding it into `actor/setter`. It has **two**
users — `channel/channel.go` and `actor/setter/setter.go`, both embedding it
and overriding `Register`. Folding it in would leave `channel` importing the
setter package for a map type.

### Setter enum registration

`Setters.Register` contributes each name to the schema as it registers, rather
than the schema reading the registry later. The original lazy-func-value
approach *did* work (verified), but its correctness depended on noticing both
the laziness and that the map is never reassigned — too subtle. Go runs a
package's init functions in **filename order**, so a setter in a file sorting
after `setter.go` registers later; any up-front snapshot would omit it. Pinned
by `actor/setter/setter_test.go`.

Note the lock discipline: `RegisterEnumValue` uses a **generation counter**
rather than resetting a `sync.Once`, because clearing the definitions cache
from there would take the definitions lock while holding the enum lock — the
opposite order from `Definitions()`, and a deadlock.

---

## Bugs found and fixed along the way

1. **Sort order zero treated as "unset".** `UpsertChannel` appended when
   `SortOrder == 0`, so `pexels-curated` — listed *first* in `active-channels`
   — imported *last*. `store.AppendOrder` (-1) now signals appending
   explicitly. The same latent bug existed in `UpsertWatermarkLayer`.
2. **Nested YAML settings silently dropped.** A namespaced setting is spelled
   as a nested mapping (`daemon:` / `  interval: 60`), and `importGlobals`
   skipped every non-scalar top-level key — so a customised refresh interval
   vanished **with no warning**. It hid because the shipped example sets the
   interval to its default value. Nested mappings now flatten to dotted keys on
   import and nest again on export.
3. **Watermark `position`/`alignment` validated nowhere.** The renderer
   switched on them and silently fell through, so a typo rendered the watermark
   in the wrong corner with no diagnostic. Now validated against
   `config/enum.go`.
4. **Enum slice copied after unlocking** — a race, caught in review.
5. **In-memory SQLite shared one database.** `file::memory:?cache=shared` is
   process-global; tests could not isolate. Now a unique name per `Open`.

---

## Verification

`make test` passes, including under `-race`; `go vet` clean on darwin and
windows. 12 test files — the repository's first substantial suite. Coverage:
store CRUD and constraints, migration idempotency / well-formedness / the
forward-dated refusal, content-schema versioning, template precedence and
inlining, dotted-path option lookup, option-type logging, YAML import of the
shipped example, nested and legacy key mapping, export round-trip stability,
setter enum ordering, auth rejection and logging, redaction, and the
400/404/405 split.

**Always verify against a throwaway app directory:**

```bash
export GOTAPAPER_APP_DIR=$(mktemp -d)
```

Manual checks that mattered:

- Loopback bind confirmed with `lsof -iTCP -sTCP:LISTEN`: `127.0.0.1:…`, **not
  `0.0.0.0`**. Worth re-checking after any server change — it is the one
  mistake here that would be genuinely harmful.
- `401` without a token, warning logged; API key stored intact but never served.
- `404`+JSON unknown path, `405`+`Allow` wrong verb, `400`+JSON unknown key.
- Example-config import yields **two** templates with both unsplash channels
  *referencing* rather than duplicating the shared key; order and probabilities
  match `active-channels`.
- Template edit propagates to both dependants; in-use delete refused by name;
  `--inline` succeeds with options preserved.
- Full `channel → crop → watermark → write → setter` pipeline sets a real
  wallpaper; dedup skips on rerun; `--force` re-downloads.
- Daemon logs `config change triggers refresh` on a change from a second shell
  (needs `--debug`; the line is debug-level). Endpoint file cleaned on SIGTERM.
- `--debug` affects the run without persisting. Clean install with no YAML.

### CI note

The **Windows build passes.** An earlier claim that it failed pre-existing was
wrong — that was a local cross-compile artifact from `GOOS=windows go build` on
darwin, where `install/install.go` fails against the pinned `go-autostart`. CI
builds on a Windows host and is the authority. Local
`CGO_ENABLED=0 GOOS=linux` also fails, because the tray needs cgo; real builds
use `gox -cgo`.

---

## Known gaps / follow-ups

- **Channels key history by their *type* constant, not instance name**
  (`ng-photo-of-today`, not `ng`) — pre-existing behaviour, preserved. Two
  instances of one type share a history entry, and `DeleteChannel`'s history
  cleanup only matches when key equals type. Changing it would alter dedup
  behaviour, so it was left alone deliberately.
- **`api/client` wraps only the settings endpoints.** TODO at the bottom of
  `client.go` names the rest (channels, templates, watermark layers, history,
  refresh). The commands that still write directly are correct but do not give
  the daemon its immediate-reaction signal.
- **No content-migration framework** — only version identification. Build it
  when a channel actually changes its option shape.
- **Linux still uses the dotfile directory**, not `XDG_CONFIG_HOME`.
- **`download_history` is one row per channel**, matching the old
  `history.json` map. A gallery feature wants an append-only log — that is a
  `0002_` migration.
- `examples/config.yaml.example` and the `vfsgen` pipeline stay: the example is
  the fullest reference for every option, since it carries explanatory comments
  the database cannot. Surfaced as `config example`.
