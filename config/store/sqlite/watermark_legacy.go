package sqlite

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// MetaWatermarkLayers versions the meaning of a watermark layer's values.
//
// Version 1 is the shape before the editor (point sizes, mixed-unit margins
// and paddings); version 2 is the picture-relative one. The table holds both
// sets of columns after migration 0006; this row says which of them is live.
const MetaWatermarkLayers = "watermark-layers"

// WatermarkLayersVersion is the watermark layer version this build writes.
const WatermarkLayersVersion = 2

// checkWatermarkSchema converts the layers of a database written before the
// editor, once, and refuses one written by a newer build.
func (s *Store) checkWatermarkSchema() error {
	stored, err := s.SchemaVersion(MetaWatermarkLayers)
	if err != nil {
		return err
	}
	switch {
	case stored > WatermarkLayersVersion:
		return fmt.Errorf(
			"%w: the watermark layer schema is at version %d but this build only understands %d",
			store.ErrSchemaTooNew, stored, WatermarkLayersVersion,
		)
	case stored == WatermarkLayersVersion:
		return nil
	}

	return s.Tx(func(txs store.Store) error {
		tx := txs.(*Store)
		n, err := tx.convertLegacyWatermarks()
		if err != nil {
			return fmt.Errorf("cannot convert the watermark layers: %w", err)
		}
		if n > 0 {
			logrus.WithField("layers", n).Info("converted watermark layers to picture-relative sizes")
		}
		return tx.SetSchemaVersion(MetaWatermarkLayers, WatermarkLayersVersion)
	})
}

// convertLegacyWatermarks rewrites every layer's new columns from its old
// ones. No change event is emitted: the configuration means what it meant
// before, only expressed differently.
func (s *Store) convertLegacyWatermarks() (int, error) {
	refW := s.referenceSize(config.KeyPictureReferenceWidth, 1920)
	refH := s.referenceSize(config.KeyPictureReferenceHeight, 1080)

	rows, err := s.q().Query(`SELECT id, font, point, color, position,
		h_margin, v_margin, linespace, alignment, template,
		absolute_point, absolute_position,
		COALESCE(bg_color, ''), bg_h_throughout, bg_v_throughout, bg_paddings
		FROM watermark_layers`)
	if err != nil {
		return 0, err
	}

	type converted struct {
		id    int64
		layer store.WatermarkLayer
	}
	var all []converted
	for rows.Next() {
		var id int64
		var old store.LegacyWatermarkLayer
		var paddings string
		if err := rows.Scan(
			&id, &old.Font, &old.Point, &old.Color, &old.Position,
			&old.HMargin, &old.VMargin, &old.Linespace, &old.Alignment, &old.Template,
			&old.AbsolutePoint, &old.AbsolutePosition,
			&old.Background.Color, &old.Background.HThroughout, &old.Background.VThroughout,
			&paddings,
		); err != nil {
			rows.Close()
			return 0, err
		}
		// An undecodable list converts as no padding, rather than leaving
		// the whole database unopenable.
		_ = json.Unmarshal([]byte(paddings), &old.Background.Paddings)
		all = append(all, converted{id, store.ConvertLegacyWatermark(old, refW, refH)})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, c := range all {
		l := c.layer
		padding, _ := json.Marshal(l.Background.Padding[:])
		if _, err := s.q().Exec(
			`UPDATE watermark_layers SET
			     size = ?, color = ?, anchor = ?, x = ?, y = ?, alignment = ?,
			     linespace = ?, padding = ?
			 WHERE id = ?`,
			l.Size, l.Color, l.Anchor, l.X, l.Y, l.Alignment,
			l.Linespace, string(padding), c.id,
		); err != nil {
			return 0, err
		}
	}
	return len(all), nil
}

// referenceSize reads one of the picture reference dimensions, which the
// conversion needs to turn pixel margins into fractions.
func (s *Store) referenceSize(key string, fallback float64) float64 {
	setting, err := s.Setting(key)
	if err != nil {
		return fallback
	}
	v, err := strconv.ParseFloat(setting.Value, 64)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
