package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

const watermarkColumns = `id, sort_order, enabled, font, point, color, position,
	h_margin, v_margin, linespace, alignment, template,
	absolute_point, absolute_position,
	COALESCE(bg_color, ''), bg_h_throughout, bg_v_throughout, bg_paddings`

// WatermarkLayers lists the configured watermark overlays in render order.
func (s *Store) WatermarkLayers() ([]store.WatermarkLayer, error) {
	rows, err := s.q().Query(
		"SELECT " + watermarkColumns + " FROM watermark_layers ORDER BY sort_order",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.WatermarkLayer
	for rows.Next() {
		l, err := scanWatermarkLayer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func scanWatermarkLayer(sc scanner) (store.WatermarkLayer, error) {
	var l store.WatermarkLayer
	var paddings string

	err := sc.Scan(
		&l.ID, &l.SortOrder, &l.Enabled, &l.Font, &l.Point, &l.Color, &l.Position,
		&l.HMargin, &l.VMargin, &l.Linespace, &l.Alignment, &l.Template,
		&l.AbsolutePoint, &l.AbsolutePosition,
		&l.Background.Color, &l.Background.HThroughout, &l.Background.VThroughout,
		&paddings,
	)
	if err != nil {
		return store.WatermarkLayer{}, err
	}

	if err := json.Unmarshal([]byte(paddings), &l.Background.Paddings); err != nil {
		return store.WatermarkLayer{}, fmt.Errorf(
			"watermark layer %d has undecodable paddings: %w", l.ID, err,
		)
	}
	return l, nil
}

// UpsertWatermarkLayer creates a layer (ID zero) or updates an existing one.
//
// Position and alignment are checked against config/enum.go rather than a SQL
// CHECK constraint. Besides keeping the valid set next to the renderer that
// consumes it, this closes a real gap: the renderer switches on these strings
// and silently falls back on an unrecognised value, so a typo used to produce a
// watermark in the wrong corner with no diagnostic at all.
func (s *Store) UpsertWatermarkLayer(l store.WatermarkLayer) error {
	if !config.ValidPosition(l.Position) {
		return &config.InvalidValueError{
			Key: "watermark.position", Value: l.Position,
			Kind: config.KindEnum, Expected: config.PositionValues,
		}
	}
	if !config.ValidAlignment(l.Alignment) {
		return &config.InvalidValueError{
			Key: "watermark.alignment", Value: l.Alignment,
			Kind: config.KindEnum, Expected: config.AlignmentValues,
		}
	}

	paddings := l.Background.Paddings
	if paddings == nil {
		paddings = []float64{}
	}
	encodedPaddings, err := json.Marshal(paddings)
	if err != nil {
		return fmt.Errorf("cannot encode watermark paddings: %w", err)
	}

	if l.ID == 0 {
		sortOrder := l.SortOrder
		if sortOrder < 0 {
			// As with channels, only store.AppendOrder appends; zero is the
			// first layer, not an absent value.
			if sortOrder, err = s.nextWatermarkSortOrder(); err != nil {
				return err
			}
		}
		_, err = s.q().Exec(
			`INSERT INTO watermark_layers
			     (sort_order, enabled, font, point, color, position,
			      h_margin, v_margin, linespace, alignment, template,
			      absolute_point, absolute_position,
			      bg_color, bg_h_throughout, bg_v_throughout, bg_paddings)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sortOrder, l.Enabled, l.Font, l.Point, l.Color, l.Position,
			l.HMargin, l.VMargin, l.Linespace, l.Alignment, l.Template,
			l.AbsolutePoint, l.AbsolutePosition,
			nullIfEmpty(l.Background.Color), l.Background.HThroughout,
			l.Background.VThroughout, string(encodedPaddings),
		)
	} else {
		var res sql.Result
		res, err = s.q().Exec(
			`UPDATE watermark_layers SET
			     sort_order = ?, enabled = ?, font = ?, point = ?, color = ?, position = ?,
			     h_margin = ?, v_margin = ?, linespace = ?, alignment = ?, template = ?,
			     absolute_point = ?, absolute_position = ?,
			     bg_color = ?, bg_h_throughout = ?, bg_v_throughout = ?, bg_paddings = ?
			 WHERE id = ?`,
			l.SortOrder, l.Enabled, l.Font, l.Point, l.Color, l.Position,
			l.HMargin, l.VMargin, l.Linespace, l.Alignment, l.Template,
			l.AbsolutePoint, l.AbsolutePosition,
			nullIfEmpty(l.Background.Color), l.Background.HThroughout,
			l.Background.VThroughout, string(encodedPaddings),
			l.ID,
		)
		if err == nil {
			if n, rerr := res.RowsAffected(); rerr == nil && n == 0 {
				return fmt.Errorf("%w: watermark layer %d", store.ErrNotFound, l.ID)
			}
		}
	}
	if err != nil {
		return err
	}

	s.emit("watermark", nil, nil)
	return nil
}

func (s *Store) nextWatermarkSortOrder() (int, error) {
	var max sql.NullInt64
	if err := s.q().QueryRow("SELECT MAX(sort_order) FROM watermark_layers").Scan(&max); err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}

// DeleteWatermarkLayer removes one layer by ID.
func (s *Store) DeleteWatermarkLayer(id int64) error {
	res, err := s.q().Exec("DELETE FROM watermark_layers WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: watermark layer %d", store.ErrNotFound, id)
	}

	s.emit("watermark", nil, nil)
	return nil
}
