-- Wallpaper history: one row per applied wallpaper, read by the UI and the
-- CLI, and capped by a count rather than overwritten.
--
-- It is also the deduplication record: a channel skips a picture whose URL
-- matches its own newest row here. That made download_history, which held one
-- URL per channel, redundant, so 0003 carries its rows over and drops it.
CREATE TABLE wallpaper_history (
    id            INTEGER PRIMARY KEY,

    -- The channel instance name, and the registry name of the implementation
    -- behind it. Both are recorded because a history row must stay readable
    -- after the channel it came from is renamed or deleted; there is
    -- deliberately no foreign key to channel_instances for the same reason.
    channel_name  TEXT NOT NULL,
    channel_type  TEXT NOT NULL DEFAULT '',

    -- Where the picture came from, kept so it can be downloaded again and
    -- re-applied. Providers that serve time-limited URLs mean this is not
    -- guaranteed to still resolve; that is accepted.
    url           TEXT NOT NULL DEFAULT '',

    title         TEXT,
    caption       TEXT,
    credit        TEXT,
    format        TEXT,

    upload_time   INTEGER,
    download_time INTEGER NOT NULL DEFAULT (unixepoch()),

    -- Where the wallpaper was written. Note this is the same path on every
    -- row: the wallpaper file is overwritten on each refresh, so only the
    -- newest row's file still holds the picture it describes. thumb_path is
    -- per-row and is what makes an older entry previewable.
    file_path     TEXT,
    thumb_path    TEXT
);

-- The history is read newest-first and pruned oldest-first, so both the list
-- query and the sweep are index scans rather than sorts over the table.
CREATE INDEX idx_wallpaper_history_download_time
    ON wallpaper_history(download_time DESC, id DESC);

-- Deleting a channel deletes its history, so this supports that too.
CREATE INDEX idx_wallpaper_history_channel ON wallpaper_history(channel_name);
