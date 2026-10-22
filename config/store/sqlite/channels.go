package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config/store"
)

// Option keys promoted to real columns on channel_instances because every
// channel reads them.
const (
	optStrategy = "strategy"
	optWidth    = "width"
	optHeight   = "height"
	optType     = "type"
)

const channelColumns = `id, name, type, template_name,
	enabled, probability, sort_order, strategy, width, height, options,
	options_version`

// Channels lists every configured channel in refresh order, enabled or not.
func (s *Store) Channels() ([]store.ChannelInstance, error) {
	rows, err := s.q().Query(
		"SELECT " + channelColumns + " FROM channel_with_template ORDER BY sort_order",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.ChannelInstance
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Channel returns one channel by name.
func (s *Store) Channel(name string) (store.ChannelInstance, error) {
	row := s.q().QueryRow(
		"SELECT "+channelColumns+" FROM channel_with_template WHERE name = ?", name,
	)
	c, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return store.ChannelInstance{}, fmt.Errorf("%w: channel %q", store.ErrNotFound, name)
	}
	return c, err
}

func scanChannel(sc scanner) (store.ChannelInstance, error) {
	var c store.ChannelInstance
	var templateName sql.NullString
	var strategy sql.NullString
	var width, height sql.NullInt64
	var options string

	err := sc.Scan(
		&c.ID, &c.Name, &c.Type, &templateName,
		&c.Enabled, &c.Probability, &c.SortOrder,
		&strategy, &width, &height, &options, &c.OptionsVersion,
	)
	if err != nil {
		return store.ChannelInstance{}, err
	}

	c.TemplateName = templateName.String
	if strategy.Valid {
		v := strategy.String
		c.Strategy = &v
	}
	if width.Valid {
		v := int(width.Int64)
		c.Width = &v
	}
	if height.Valid {
		v := int(height.Int64)
		c.Height = &v
	}

	opts, err := decodeOptions(options)
	if err != nil {
		return store.ChannelInstance{}, fmt.Errorf("channel %q: %w", c.Name, err)
	}
	c.Options = opts
	return c, nil
}

// EffectiveChannels returns the enabled channels, in order, with their
// templates already applied.
func (s *Store) EffectiveChannels() ([]store.EffectiveChannel, error) {
	channels, err := s.Channels()
	if err != nil {
		return nil, err
	}

	var out []store.EffectiveChannel
	for _, c := range channels {
		if !c.Enabled {
			continue
		}
		eff, err := s.resolve(c)
		if err != nil {
			return nil, err
		}
		out = append(out, eff)
	}
	return out, nil
}

// EffectiveChannel resolves one channel regardless of whether it is enabled,
// so an explicit request on the command line can still run a disabled channel.
func (s *Store) EffectiveChannel(name string) (store.EffectiveChannel, error) {
	c, err := s.Channel(name)
	if err != nil {
		return store.EffectiveChannel{}, err
	}
	return s.resolve(c)
}

// resolve layers a channel's configuration the way a YAML merge key did:
// template options first, then the instance's own options, then its promoted
// columns. Later layers win.
//
// This runs in Go rather than in SQL on purpose. The obvious SQL spelling,
// json_patch(template, json_object('strategy', c.strategy, ...)), is wrong:
// json_patch treats a null member as "delete this key", so a NULL instance
// column would erase the template's value instead of inheriting it -- the
// exact opposite of what NULL means here.
func (s *Store) resolve(c store.ChannelInstance) (store.EffectiveChannel, error) {
	options := map[string]any{}

	if c.TemplateName != "" {
		t, err := s.ChannelTemplate(c.TemplateName)
		if err != nil {
			return store.EffectiveChannel{}, fmt.Errorf(
				"channel %q: cannot load template %q: %w", c.Name, c.TemplateName, err,
			)
		}
		for k, v := range t.Options {
			options[k] = v
		}
	}

	for k, v := range c.Options {
		options[k] = v
	}

	// Promoted columns are the most specific layer. A nil pointer means
	// "inherit", so it must leave whatever the lower layers supplied in place.
	if c.Strategy != nil {
		options[optStrategy] = *c.Strategy
	}
	if c.Width != nil {
		options[optWidth] = *c.Width
	}
	if c.Height != nil {
		options[optHeight] = *c.Height
	}

	// The channel's registry type is not user data, so it always wins; a
	// template must not be able to redirect a channel to another provider.
	options[optType] = c.Type

	return store.EffectiveChannel{
		ID:          c.ID,
		Name:        c.Name,
		Type:        c.Type,
		Enabled:     c.Enabled,
		Probability: c.Probability,
		SortOrder:   c.SortOrder,
		Options:     options,
	}, nil
}

// UpsertChannel creates or replaces a channel, keyed by name.
func (s *Store) UpsertChannel(c store.ChannelInstance) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("channel name cannot be empty")
	}
	if strings.TrimSpace(c.Type) == "" {
		return fmt.Errorf("channel %q must have a type", c.Name)
	}
	if c.Probability < 0 || c.Probability > 1 {
		return fmt.Errorf(
			"channel %q: probability %v is outside [0, 1]", c.Name, c.Probability,
		)
	}

	options, err := encodeOptions(c.Options)
	if err != nil {
		return err
	}

	var templateID any
	if c.TemplateName != "" {
		t, err := s.ChannelTemplate(c.TemplateName)
		if err != nil {
			return err
		}
		templateID = t.ID
	}

	sortOrder := c.SortOrder
	if sortOrder < 0 {
		// Only an explicit store.AppendOrder appends. Zero must stay a real
		// position, or a caller placing a channel first would silently get it
		// moved to the end.
		if sortOrder, err = s.nextSortOrder(); err != nil {
			return err
		}
	}

	// Stamp with the live schema version of the channel's type, so a future
	// build can tell which option shape these values were written against.
	optionsVersion := c.OptionsVersion
	if optionsVersion == 0 {
		optionsVersion = currentOptionsVersion(c.Type)
	}

	_, err = s.q().Exec(
		`INSERT INTO channel_instances
		     (name, type, template_id, enabled, probability, sort_order,
		      strategy, width, height, options, options_version, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
		 ON CONFLICT(name) DO UPDATE SET
		     type = excluded.type,
		     template_id = excluded.template_id,
		     enabled = excluded.enabled,
		     probability = excluded.probability,
		     sort_order = excluded.sort_order,
		     strategy = excluded.strategy,
		     width = excluded.width,
		     height = excluded.height,
		     options = excluded.options,
		     options_version = excluded.options_version,
		     updated_at = excluded.updated_at`,
		c.Name, c.Type, templateID, c.Enabled, c.Probability, sortOrder,
		ptrToAny(c.Strategy), ptrToAny(c.Width), ptrToAny(c.Height), options,
		optionsVersion,
	)
	if err != nil {
		return err
	}

	s.emit("channels."+c.Name, nil, c.Name)
	return nil
}

func (s *Store) nextSortOrder() (int, error) {
	var max sql.NullInt64
	err := s.q().QueryRow("SELECT MAX(sort_order) FROM channel_instances").Scan(&max)
	if err != nil {
		return 0, err
	}
	if !max.Valid {
		return 0, nil
	}
	return int(max.Int64) + 1, nil
}

// DeleteChannel removes a channel and its download history.
func (s *Store) DeleteChannel(name string) error {
	return s.Tx(func(tx store.Store) error {
		inner := tx.(*Store)

		res, err := inner.q().Exec("DELETE FROM channel_instances WHERE name = ?", name)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err == nil && n == 0 {
			return fmt.Errorf("%w: channel %q", store.ErrNotFound, name)
		}

		// History is keyed by channel name, not by a foreign key, so it would
		// otherwise linger and suppress a redownload if the channel were
		// recreated under the same name later.
		if _, err := inner.q().Exec(
			"DELETE FROM download_history WHERE channel_name = ?", name,
		); err != nil {
			return err
		}

		inner.emit("channels."+name, name, nil)
		return nil
	})
}

// ReorderChannels sets sort_order to match the given name order. Every
// configured channel must be named, so a partial list cannot silently leave
// channels sharing a position.
func (s *Store) ReorderChannels(names []string) error {
	return s.Tx(func(tx store.Store) error {
		inner := tx.(*Store)

		existing, err := inner.Channels()
		if err != nil {
			return err
		}
		if len(names) != len(existing) {
			return fmt.Errorf(
				"reorder needs all %d channels, got %d", len(existing), len(names),
			)
		}

		known := map[string]bool{}
		for _, c := range existing {
			known[c.Name] = true
		}
		for _, name := range names {
			if !known[name] {
				return fmt.Errorf("%w: channel %q", store.ErrNotFound, name)
			}
			delete(known, name)
		}
		if len(known) > 0 {
			return fmt.Errorf("reorder omitted channels: %v", mapKeys(known))
		}

		for i, name := range names {
			if _, err := inner.q().Exec(
				"UPDATE channel_instances SET sort_order = ?, updated_at = unixepoch() WHERE name = ?",
				i, name,
			); err != nil {
				return err
			}
		}

		inner.emit("channels", nil, strings.Join(names, ","))
		return nil
	})
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ptrToAny converts a typed nil pointer into an untyped nil so the driver
// stores SQL NULL rather than a zero value.
func ptrToAny[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// asString coerces an option value to a string.
//
// Both numeric Go types and float64 are handled because an options map reaches
// this code by two routes: decoded from stored JSON, where every number is a
// float64, or passed straight in by a caller, where 1920 is an int.
func asString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case int:
		return strconv.Itoa(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case bool:
		return strconv.FormatBool(t), true
	default:
		return "", false
	}
}

// asInt coerces an option value to an int.
func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case string:
		i, err := strconv.Atoi(t)
		return i, err == nil
	default:
		return 0, false
	}
}

// currentOptionsVersion reports the option-schema version a channel type
// declares, defaulting to 1 for a type that is not registered in this build
// (an instance configured for a channel that has since been removed, say).
func currentOptionsVersion(channelType string) int {
	if schema, ok := channel.Channels.Schema(channelType); ok {
		return schema.SchemaVersion()
	}
	return 1
}
