-- goTApaper configuration schema, initial version.
--
-- Design note: this schema carries only *structural* constraints -- primary
-- keys, foreign keys, JSON validity and numeric invariants. Enumerated value
-- sets (crop modes, watermark positions and alignments, setter names, download
-- strategies) are deliberately NOT expressed as CHECK (... IN (...)) lists.
-- They live in Go, next to the code that consumes them, because:
--   * the valid `setter` values vary per OS via build tags, so no fixed list
--     here could be correct on every platform;
--   * each channel type accepts a different subset of strategies;
--   * adding a value should be a code change, not a schema migration.
-- The store validates against config.Definitions and config/enum.go on write.

-- Content schema versions, distinct from PRAGMA user_version.
--
-- user_version covers table *structure* and is what the migration runner
-- tracks. These cover the *meaning* of what is stored: the set of setting
-- keys, and the option shape each channel type expects. An older build can
-- then recognise a configuration written by a newer one and refuse it, rather
-- than silently ignoring keys it does not understand and operating on a
-- half-understood configuration.
--
-- They are kept as rows rather than more PRAGMAs because content migrations
-- are not always expressible in SQL: renaming an option or splitting one
-- value into two may need Go code, so the version has to be readable and
-- writable from Go at any point, not only during a migration.
CREATE TABLE schema_meta (
    name       TEXT PRIMARY KEY,
    version    INTEGER NOT NULL,
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Reusable blocks of channel options: the direct replacement for a YAML anchor
-- merged into a channel with "<<: *name". A template has no identity of its
-- own; it is never refreshed, only inherited from.
CREATE TABLE channel_templates (
    id         INTEGER PRIMARY KEY,
    name       TEXT NOT NULL UNIQUE,
    type       TEXT,
    options    TEXT NOT NULL DEFAULT '{}'
                 CHECK (json_valid(options)),
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Global scalar settings. Values only: kind, default, enum membership and
-- secrecy all come from config.Definitions. A key stored here but absent from
-- Definitions is logged and ignored, so a database written by a newer build
-- does not break an older one.
CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT,
    updated_at INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Channel instances. Replaces both `channels:` and `active-channels:`.
CREATE TABLE channel_instances (
    id          INTEGER PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,

    -- NULLable: a channel need not inherit from a template.
    -- RESTRICT rather than SET NULL: deleting a template that is still in use
    -- would silently strip its channels of their inherited API key and
    -- dimensions, leaving a config that looks fine and fails at download time.
    template_id INTEGER NULL REFERENCES channel_templates(id) ON DELETE RESTRICT,

    enabled     INTEGER NOT NULL DEFAULT 1,

    -- A structural invariant rather than a policy list, so it belongs in SQL.
    probability REAL NOT NULL DEFAULT 1.0
                  CHECK (probability >= 0.0 AND probability <= 1.0),

    sort_order  INTEGER NOT NULL,

    -- Promoted out of `options` because every channel reads them.
    -- NULL means "inherit from the template".
    strategy    TEXT,
    width       INTEGER,
    height      INTEGER,

    -- Instance-specific options (key, query, orientation, featured, dpr,
    -- image_parameters, meta.*), overriding the template key by key.
    options     TEXT NOT NULL DEFAULT '{}'
                  CHECK (json_valid(options)),

    -- The option-schema version these options were written against, so a
    -- channel that changes its option shape can tell an old row from a new
    -- one and migrate it in Go. Upgrading a downloader is exactly the case
    -- plain SQL cannot handle.
    options_version INTEGER NOT NULL DEFAULT 1,

    updated_at  INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX idx_channel_enabled  ON channel_instances(enabled, sort_order);
CREATE INDEX idx_channel_template ON channel_instances(template_id);

-- Channels with their template resolved to a name, which is what every read
-- actually wants: the Go layer works in template names, not row ids. Keeping
-- the join here means the query in channels.go is a plain column list instead
-- of a correlated subquery repeated at each call site.
--
-- Note this resolves the template *reference* only, not the inherited option
-- values. Option layering stays in Go: the obvious SQL spelling would overlay
-- the promoted columns with json_patch, and json_patch treats a null member as
-- "delete this key", so a NULL column would erase the template's value rather
-- than inherit it.
CREATE VIEW channel_with_template AS
SELECT c.id,
       c.name,
       c.type,
       t.name AS template_name,
       c.enabled,
       c.probability,
       c.sort_order,
       c.strategy,
       c.width,
       c.height,
       c.options,
       c.options_version,
       c.updated_at
FROM channel_instances c
LEFT JOIN channel_templates t ON t.id = c.template_id;

-- Watermark layers: an ordered list a UI will reorder, extend and delete from.
-- `position` and `alignment` are validated in Go against config/enum.go.
-- `template` here is a Go text/template source string, unrelated to
-- channel_templates.
CREATE TABLE watermark_layers (
    id                INTEGER PRIMARY KEY,
    sort_order        INTEGER NOT NULL,
    enabled           INTEGER NOT NULL DEFAULT 1,
    font              TEXT    NOT NULL DEFAULT 'NotoSans-Regular.ttf',
    point             REAL    NOT NULL DEFAULT 13,
    color             TEXT    NOT NULL DEFAULT '222222',
    position          TEXT    NOT NULL DEFAULT 'bottom-center',
    h_margin          REAL    NOT NULL DEFAULT 0.1,
    v_margin          REAL    NOT NULL DEFAULT 0.1,
    linespace         REAL    NOT NULL DEFAULT 1.4,
    alignment         TEXT    NOT NULL DEFAULT 'center',
    template          TEXT    NOT NULL DEFAULT '',
    absolute_point    INTEGER NOT NULL DEFAULT 0,
    absolute_position INTEGER NOT NULL DEFAULT 0,
    bg_color          TEXT,
    bg_h_throughout   INTEGER NOT NULL DEFAULT 0,
    bg_v_throughout   INTEGER NOT NULL DEFAULT 0,
    bg_paddings       TEXT    NOT NULL DEFAULT '[0.2]'
                        CHECK (json_valid(bg_paddings))
);

-- Download history: one row per channel, mirroring history.json's
-- map[string]History exactly. Mark overwrites, Has is an equality check. This
-- is not a log -- the primary key on channel_name is what enforces that.
CREATE TABLE download_history (
    channel_name  TEXT PRIMARY KEY,
    url           TEXT NOT NULL,
    title         TEXT,
    credit        TEXT,
    upload_time   INTEGER,
    download_time INTEGER NOT NULL DEFAULT (unixepoch()),
    file_path     TEXT
);

-- Seed two channels so a fresh install downloads a wallpaper with no further
-- configuration. These are real user-editable rows, unlike the global setting
-- defaults, which stay in code.
INSERT INTO channel_instances (name, type, strategy, sort_order) VALUES
    ('ng',   'ng-photo-of-today', NULL,              0),
    ('bing', 'bing-wallpaper',    'largest-no-logo', 1);
