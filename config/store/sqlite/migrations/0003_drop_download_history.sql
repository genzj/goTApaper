-- Deduplication moves onto wallpaper_history, and download_history goes.
--
-- download_history held one URL per channel -- a copy of what is now just the
-- newest wallpaper_history row for that channel, and a copy that went stale in
-- its own ways. It was keyed by channel *type* ("bing-wallpaper"), so two
-- instances of one type shared an entry, and it was written at download time,
-- so a picture that then failed to apply still counted as seen.
--
-- Its rows are carried over before the drop, so the first refresh after the
-- upgrade does not download the current picture again. A key is attributed the
-- same way yamlimport.attribute does it: an instance with exactly that name
-- wins; otherwise the key is read as a type, which identifies an instance only
-- when exactly one instance has it. Anything else is dropped, costing at most
-- one redundant download per channel.
INSERT INTO wallpaper_history
    (channel_name, channel_type, url, title, credit, upload_time,
     download_time, file_path)
SELECT ci.name, ci.type, dh.url, dh.title, dh.credit, dh.upload_time,
       dh.download_time, dh.file_path
FROM download_history dh
JOIN channel_instances ci
  ON ci.name = dh.channel_name
  OR (ci.type = dh.channel_name
      AND NOT EXISTS (
          SELECT 1 FROM channel_instances named WHERE named.name = dh.channel_name)
      AND (SELECT COUNT(*) FROM channel_instances same
           WHERE same.type = dh.channel_name) = 1)
WHERE dh.url != '';

DROP TABLE download_history;
