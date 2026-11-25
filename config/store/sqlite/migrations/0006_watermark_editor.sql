-- Picture-relative watermark layers, for the watermark editor.
--
-- The old columns mixed units: `point` was scaled by the picture height over
-- the reference height unless `absolute_point`; margins and paddings were
-- fractions below 1 and pixels from 1; centre positions ignored margins. The
-- new columns are all relative to the picture:
--
--   size     font size, % of the picture's shorter side
--   anchor   which point of the text box x and y place
--   x, y     that point, as fractions of the picture's width and height
--   padding  [top, right, bottom, left] in em (multiples of the font size)
--   shadow_dx/dy, outline_width in em
--
-- Existing rows are converted by Go code after this migration (sqlite
-- convertLegacyWatermarks), because the conversion reads the reference size
-- from the settings and needs arithmetic SQL would make unreadable. It is
-- tracked by the `watermark-layers` row in schema_meta. The old columns are
-- left in place, unused, so a later migration can drop them once no database
-- still needs converting.
ALTER TABLE watermark_layers ADD COLUMN name          TEXT NOT NULL DEFAULT '';
ALTER TABLE watermark_layers ADD COLUMN size          REAL NOT NULL DEFAULT 1.6;
ALTER TABLE watermark_layers ADD COLUMN anchor        TEXT NOT NULL DEFAULT 'bottom-right';
ALTER TABLE watermark_layers ADD COLUMN x             REAL NOT NULL DEFAULT 0.97;
ALTER TABLE watermark_layers ADD COLUMN y             REAL NOT NULL DEFAULT 0.95;
ALTER TABLE watermark_layers ADD COLUMN padding       TEXT NOT NULL DEFAULT '[0,0,0,0]'
                                                      CHECK (json_valid(padding));
ALTER TABLE watermark_layers ADD COLUMN shadow_color  TEXT;
ALTER TABLE watermark_layers ADD COLUMN shadow_dx     REAL NOT NULL DEFAULT 0;
ALTER TABLE watermark_layers ADD COLUMN shadow_dy     REAL NOT NULL DEFAULT 0;
ALTER TABLE watermark_layers ADD COLUMN outline_color TEXT;
ALTER TABLE watermark_layers ADD COLUMN outline_width REAL NOT NULL DEFAULT 0;
