-- One history entry per picture.
--
-- A forced refresh downloads a picture the channel already applied, and each
-- one appended a duplicate entry. A picture applied again -- re-applied by hand,
-- or downloaded again by a forced refresh -- should update its entry instead,
-- as a re-apply already did: last applied now, first applied kept, count up.
-- Pictures are identified by their source URL. Two different URLs for one
-- picture still make two entries: nothing compares the images themselves.
--
-- The duplicates already recorded are merged into the newest entry of each
-- URL, which is the one deduplication already compares against. It keeps the
-- earliest first application, the sum of the counts, and any title, caption,
-- credit, upload time or thumbnail it lacks from the entries merged into it.
-- The thumbnails of the merged entries are left for the orphan sweep.
CREATE TEMP TABLE merged_history AS
SELECT url,
       (SELECT w2.id FROM wallpaper_history w2
        WHERE w2.url = w.url
        ORDER BY w2.download_time DESC, w2.id DESC LIMIT 1)   AS keep_id,
       MIN(COALESCE(first_applied_time, download_time))       AS first_time,
       SUM(times_applied)                                     AS times,
       MAX(title)                                             AS any_title,
       MAX(caption)                                           AS any_caption,
       MAX(credit)                                            AS any_credit,
       MAX(upload_time)                                       AS any_upload_time,
       MAX(thumb_path)                                        AS any_thumb
FROM wallpaper_history w
WHERE url != ''
GROUP BY url
HAVING COUNT(*) > 1;

UPDATE wallpaper_history SET
    first_applied_time = (SELECT first_time FROM merged_history m WHERE m.keep_id = wallpaper_history.id),
    times_applied      = (SELECT times      FROM merged_history m WHERE m.keep_id = wallpaper_history.id),
    title       = COALESCE(title,       (SELECT any_title       FROM merged_history m WHERE m.keep_id = wallpaper_history.id)),
    caption     = COALESCE(caption,     (SELECT any_caption     FROM merged_history m WHERE m.keep_id = wallpaper_history.id)),
    credit      = COALESCE(credit,      (SELECT any_credit      FROM merged_history m WHERE m.keep_id = wallpaper_history.id)),
    upload_time = COALESCE(upload_time, (SELECT any_upload_time FROM merged_history m WHERE m.keep_id = wallpaper_history.id)),
    thumb_path  = COALESCE(thumb_path,  (SELECT any_thumb       FROM merged_history m WHERE m.keep_id = wallpaper_history.id))
WHERE id IN (SELECT keep_id FROM merged_history);

DELETE FROM wallpaper_history
WHERE url IN (SELECT url FROM merged_history)
  AND id NOT IN (SELECT keep_id FROM merged_history);

DROP TABLE merged_history;

-- Enforced from here on. Entries with no URL -- legacy imports that kept none
-- -- are exempt: they cannot be told apart, and cannot be re-applied either.
CREATE UNIQUE INDEX idx_wallpaper_history_url ON wallpaper_history(url) WHERE url != '';
