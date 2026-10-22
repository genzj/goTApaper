package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/genzj/goTApaper/config/store"
)

// ChannelTemplates lists every template, by name.
func (s *Store) ChannelTemplates() ([]store.ChannelTemplate, error) {
	rows, err := s.q().Query(
		"SELECT id, name, COALESCE(type, ''), options FROM channel_templates ORDER BY name",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.ChannelTemplate
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ChannelTemplate returns one template by name.
func (s *Store) ChannelTemplate(name string) (store.ChannelTemplate, error) {
	row := s.q().QueryRow(
		"SELECT id, name, COALESCE(type, ''), options FROM channel_templates WHERE name = ?",
		name,
	)
	t, err := scanTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return store.ChannelTemplate{}, fmt.Errorf("%w: channel template %q", store.ErrNotFound, name)
	}
	return t, err
}

// scanner covers *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanTemplate(sc scanner) (store.ChannelTemplate, error) {
	var t store.ChannelTemplate
	var options string
	if err := sc.Scan(&t.ID, &t.Name, &t.Type, &options); err != nil {
		return store.ChannelTemplate{}, err
	}
	opts, err := decodeOptions(options)
	if err != nil {
		return store.ChannelTemplate{}, fmt.Errorf("channel template %q: %w", t.Name, err)
	}
	t.Options = opts
	return t, nil
}

// UpsertChannelTemplate creates or replaces a template, keyed by name.
func (s *Store) UpsertChannelTemplate(t store.ChannelTemplate) error {
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("channel template name cannot be empty")
	}

	options, err := encodeOptions(t.Options)
	if err != nil {
		return err
	}

	_, err = s.q().Exec(
		`INSERT INTO channel_templates (name, type, options, updated_at)
		 VALUES (?, ?, ?, unixepoch())
		 ON CONFLICT(name) DO UPDATE SET
		     type = excluded.type,
		     options = excluded.options,
		     updated_at = excluded.updated_at`,
		t.Name, nullIfEmpty(t.Type), options,
	)
	if err != nil {
		return err
	}

	// A template edit changes the effective config of every channel that
	// inherits it, so each one gets its own event. A listener watching
	// "channels.bing.*" should not have to know templates exist.
	dependents, err := s.ChannelsUsingTemplate(t.Name)
	if err != nil {
		return err
	}
	s.emit("channel-templates."+t.Name, nil, t.Options)
	for _, name := range dependents {
		s.emit("channels."+name, nil, nil)
	}
	return nil
}

// ChannelsUsingTemplate lists the channels inheriting from a template.
func (s *Store) ChannelsUsingTemplate(name string) ([]string, error) {
	rows, err := s.q().Query(
		`SELECT c.name FROM channel_instances c
		 JOIN channel_templates t ON t.id = c.template_id
		 WHERE t.name = ? ORDER BY c.sort_order`,
		name,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// DeleteChannelTemplate removes a template that nothing references.
//
// A template still in use is refused rather than cascaded. Clearing the
// reference instead would strip those channels of their inherited API key and
// dimensions, leaving a configuration that still looks complete and fails only
// at download time. Callers who really want the template gone should use
// InlineChannelTemplate.
func (s *Store) DeleteChannelTemplate(name string) error {
	dependents, err := s.ChannelsUsingTemplate(name)
	if err != nil {
		return err
	}
	if len(dependents) > 0 {
		return fmt.Errorf(
			"%w: %q is inherited by %s; inline it first to keep their settings",
			store.ErrTemplateInUse, name, strings.Join(dependents, ", "),
		)
	}

	res, err := s.q().Exec("DELETE FROM channel_templates WHERE name = ?", name)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: channel template %q", store.ErrNotFound, name)
	}

	s.emit("channel-templates."+name, nil, nil)
	return nil
}

// InlineChannelTemplate copies a template's options down into each channel
// that inherits it, clears the references, and deletes the template. The
// channels keep their effective configuration unchanged; only the sharing goes
// away. This is the deliberate escape hatch from ON DELETE RESTRICT.
func (s *Store) InlineChannelTemplate(name string) error {
	return s.Tx(func(tx store.Store) error {
		t, err := tx.ChannelTemplate(name)
		if err != nil {
			return err
		}

		dependents, err := tx.ChannelsUsingTemplate(name)
		if err != nil {
			return err
		}

		for _, channelName := range dependents {
			c, err := tx.Channel(channelName)
			if err != nil {
				return err
			}

			// Template values go underneath: the instance's own options win,
			// exactly as they did while the template was still referenced.
			merged := map[string]any{}
			for k, v := range t.Options {
				merged[k] = v
			}
			for k, v := range c.Options {
				merged[k] = v
			}

			// Promoted columns are part of the inherited set too, so fill any
			// the instance was leaving to the template.
			inheritPromoted(&c, t.Options)

			c.Options = merged
			c.TemplateName = ""
			if err := tx.UpsertChannel(c); err != nil {
				return err
			}
		}

		return tx.DeleteChannelTemplate(name)
	})
}

// inheritPromoted copies strategy, width and height out of a template's
// options into any promoted field the instance left unset.
func inheritPromoted(c *store.ChannelInstance, templateOptions map[string]any) {
	if c.Strategy == nil {
		if v, ok := templateOptions[optStrategy]; ok {
			if s, ok := asString(v); ok {
				c.Strategy = &s
			}
		}
	}
	if c.Width == nil {
		if v, ok := templateOptions[optWidth]; ok {
			if i, ok := asInt(v); ok {
				c.Width = &i
			}
		}
	}
	if c.Height == nil {
		if v, ok := templateOptions[optHeight]; ok {
			if i, ok := asInt(v); ok {
				c.Height = &i
			}
		}
	}
}

func decodeOptions(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var opts map[string]any
	if err := json.Unmarshal([]byte(raw), &opts); err != nil {
		return nil, fmt.Errorf("cannot decode stored options: %w", err)
	}
	if opts == nil {
		opts = map[string]any{}
	}
	return opts, nil
}

func encodeOptions(opts map[string]any) (string, error) {
	if opts == nil {
		return "{}", nil
	}
	bs, err := json.Marshal(opts)
	if err != nil {
		return "", fmt.Errorf("cannot encode options: %w", err)
	}
	return string(bs), nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
