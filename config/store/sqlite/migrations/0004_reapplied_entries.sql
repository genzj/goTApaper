-- Re-applying a history entry updates it rather than appending a duplicate.
--
-- download_time now means "last applied": it is what orders the history,
-- what deduplication reads the newest row by, and what pruning keeps the
-- newest of, and a picture applied again is as recent as one just downloaded.
-- first_applied_time keeps when the picture was applied for the first time;
-- NULL means it has only been applied once, so download_time is both.
ALTER TABLE wallpaper_history ADD COLUMN first_applied_time INTEGER;

-- How many times the picture has been applied, re-applications included.
ALTER TABLE wallpaper_history ADD COLUMN times_applied INTEGER NOT NULL DEFAULT 1
    CHECK (times_applied >= 1);
