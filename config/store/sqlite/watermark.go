package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

const watermarkColumns = `id, sort_order, enabled, name, template,
	font, size, color, anchor, x, y, alignment, linespace,
	COALESCE(bg_color, ''), bg_h_throughout, bg_v_throughout, padding,
	COALESCE(shadow_color, ''), shadow_dx, shadow_dy,
	COALESCE(outline_color, ''), outline_width`

// WatermarkLayers lists the configured watermark overlays in render order.
func (s *Store) WatermarkLayers() ([]store.WatermarkLayer, error) {
	rows, err := s.q().Query(
		"SELECT " + watermarkColumns + " FROM watermark_layers ORDER BY sort_order, id",
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
	var padding string

	err := sc.Scan(
		&l.ID, &l.SortOrder, &l.Enabled, &l.Name, &l.Template,
		&l.Font, &l.Size, &l.Color, &l.Anchor, &l.X, &l.Y, &l.Alignment, &l.Linespace,
		&l.Background.Color, &l.Background.HThroughout, &l.Background.VThroughout, &padding,
		&l.Shadow.Color, &l.Shadow.DX, &l.Shadow.DY,
		&l.Outline.Color, &l.Outline.Width,
	)
	if err != nil {
		return store.WatermarkLayer{}, err
	}

	// Decoded into a slice first: a fixed-size array would silently accept a
	// stored list of the wrong length.
	var p []float64
	if err := json.Unmarshal([]byte(padding), &p); err != nil || len(p) != 4 {
		return store.WatermarkLayer{}, fmt.Errorf(
			"watermark layer %d has undecodable padding %q", l.ID, padding,
		)
	}
	copy(l.Background.Padding[:], p)
	return l, nil
}

// UpsertWatermarkLayer creates a layer (ID zero) or updates an existing one.
//
// Every field is checked by store.ValidateWatermarkLayer rather than a SQL
// CHECK constraint. Besides keeping the valid set next to the renderer that
// consumes it, this closes a real gap: the renderer used to fall back silently
// on an unrecognised value, so a typo produced a watermark in the wrong corner
// with no diagnostic at all.
func (s *Store) UpsertWatermarkLayer(l store.WatermarkLayer) error {
	if err := s.writeWatermarkLayer(l); err != nil {
		return err
	}
	s.emit("watermark", nil, nil)
	return nil
}

// writeWatermarkLayer validates and stores one layer without announcing it,
// so ReplaceWatermarkLayers can announce a whole list once.
func (s *Store) writeWatermarkLayer(l store.WatermarkLayer) error {
	if err := store.ValidateWatermarkLayer(l); err != nil {
		return err
	}

	encodedPadding, err := json.Marshal(l.Background.Padding[:])
	if err != nil {
		return fmt.Errorf("cannot encode watermark padding: %w", err)
	}

	args := []any{
		l.Enabled, l.Name, l.Template,
		l.Font, l.Size, l.Color, l.Anchor, l.X, l.Y, l.Alignment, l.Linespace,
		nullIfEmpty(l.Background.Color), l.Background.HThroughout,
		l.Background.VThroughout, string(encodedPadding),
		nullIfEmpty(l.Shadow.Color), l.Shadow.DX, l.Shadow.DY,
		nullIfEmpty(l.Outline.Color), l.Outline.Width,
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
			     (sort_order, enabled, name, template,
			      font, size, color, anchor, x, y, alignment, linespace,
			      bg_color, bg_h_throughout, bg_v_throughout, padding,
			      shadow_color, shadow_dx, shadow_dy, outline_color, outline_width)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			append([]any{sortOrder}, args...)...,
		)
		return err
	}

	res, err := s.q().Exec(
		`UPDATE watermark_layers SET
		     sort_order = ?, enabled = ?, name = ?, template = ?,
		     font = ?, size = ?, color = ?, anchor = ?, x = ?, y = ?,
		     alignment = ?, linespace = ?,
		     bg_color = ?, bg_h_throughout = ?, bg_v_throughout = ?, padding = ?,
		     shadow_color = ?, shadow_dx = ?, shadow_dy = ?,
		     outline_color = ?, outline_width = ?
		 WHERE id = ?`,
		append(append([]any{l.SortOrder}, args...), l.ID)...,
	)
	if err != nil {
		return err
	}
	if n, rerr := res.RowsAffected(); rerr == nil && n == 0 {
		return fmt.Errorf("%w: watermark layer %d", store.ErrNotFound, l.ID)
	}
	return nil
}

// ReplaceWatermarkLayers replaces the whole list in one transaction, which is
// what the editor saves: a reorder, an addition and a deletion together must
// never be half-applied.
func (s *Store) ReplaceWatermarkLayers(layers []store.WatermarkLayer) ([]store.WatermarkLayer, error) {
	if len(layers) > store.MaxWatermarkLayers {
		return nil, &config.InvalidValueError{
			Key: "watermark", Value: fmt.Sprint(len(layers)),
			Kind: config.Kind(fmt.Sprintf("list of at most %d layers", store.MaxWatermarkLayers)),
		}
	}

	var out []store.WatermarkLayer
	err := s.Tx(func(txs store.Store) error {
		tx := txs.(*Store)

		existing := map[int64]bool{}
		current, err := tx.WatermarkLayers()
		if err != nil {
			return err
		}
		for _, l := range current {
			existing[l.ID] = true
		}

		kept := map[int64]bool{}
		for i, l := range layers {
			l.SortOrder = i
			if l.ID != 0 && (!existing[l.ID] || kept[l.ID]) {
				// An unknown or repeated ID is a new layer, not an error:
				// a client duplicating a layer may copy its ID along.
				l.ID = 0
			}
			if l.ID != 0 {
				kept[l.ID] = true
			}
			if err := tx.writeWatermarkLayer(l); err != nil {
				return fmt.Errorf("layer %d: %w", i+1, err)
			}
		}

		for id := range existing {
			if kept[id] {
				continue
			}
			if _, err := tx.q().Exec("DELETE FROM watermark_layers WHERE id = ?", id); err != nil {
				return err
			}
		}

		tx.emit("watermark", nil, nil)
		out, err = tx.WatermarkLayers()
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
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
