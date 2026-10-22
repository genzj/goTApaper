// Package yamlimport migrates a legacy config.yaml into the configuration
// database. It runs once, on first start, and then renames the file aside.
//
// The import deliberately walks a yaml.Node tree rather than unmarshalling
// into a map. Unmarshalling resolves anchors and merge keys, which would
// flatten the user's shared settings blocks -- their API key would be copied
// into every channel that referenced it, and editing it afterwards would mean
// editing N places. Walking the nodes lets "<<: *unsplash-common-settings"
// survive as a channel template, preserving the sharing the user set up.
package yamlimport

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// ImportIfPresent imports path into s if the file exists, then renames it.
// A missing file is not an error: a fresh install simply has nothing to
// migrate.
func ImportIfPresent(s store.Store, path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		logrus.Debug("no legacy configuration file to import")
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		logrus.WithField("file", path).Debug("legacy configuration file is empty, skipping")
		return nil
	}

	logrus.WithField("file", path).Info("importing the existing configuration file")

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if err := Import(s, data); err != nil {
		// The YAML is left exactly where it was, so the user can fix it and
		// retry, or report it.
		return fmt.Errorf("importing %s: %w", path, err)
	}

	if err := archive(path); err != nil {
		return err
	}

	// History lived next to the config and is keyed the same way.
	if err := importHistoryFile(s, filepath.Join(filepath.Dir(path), "history.json")); err != nil {
		// Losing history only costs one redundant download, so it must not
		// block a configuration import that already succeeded.
		logrus.WithError(err).Warn("cannot import the download history")
	}

	logrus.Info("configuration imported into the database")
	return nil
}

// archive renames a consumed file so it is never read again, but is still
// there if the user wants it back.
func archive(path string) error {
	target := path + ".migrated"
	if err := os.Rename(path, target); err != nil {
		return fmt.Errorf("cannot rename %s after importing it: %w", path, err)
	}
	logrus.WithField("file", target).Info("the imported file has been renamed")
	return nil
}

// Import reads a configuration document into the store in a single
// transaction, so a failure part-way leaves no half-configured database.
func Import(s store.Store, data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("cannot parse the configuration file: %w", err)
	}

	root := documentRoot(&doc)
	if root == nil {
		return fmt.Errorf("the configuration file is not a YAML mapping")
	}

	return s.Tx(func(tx store.Store) error {
		return (&importer{store: tx}).run(root)
	})
}

// documentRoot unwraps the document node to the top-level mapping.
func documentRoot(doc *yaml.Node) *yaml.Node {
	n := doc
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

type importer struct {
	store store.Store
	// templates maps a YAML anchor name to the template row created for it.
	templates map[string]string
}

func (im *importer) run(root *yaml.Node) error {
	im.templates = map[string]string{}

	// Templates first: channels reference them.
	if err := im.importTemplates(root); err != nil {
		return err
	}

	channelsNode := mappingValue(root, "channels")
	activeNode := mappingValue(root, "active-channels")
	if err := im.importChannels(channelsNode, activeNode); err != nil {
		return err
	}

	if err := im.importWatermarks(mappingValue(root, "watermark")); err != nil {
		return err
	}

	return im.importGlobals(root)
}

// knownSections are top-level keys handled structurally rather than as
// scalar settings.
var knownSections = map[string]bool{
	"channels":        true,
	"active-channels": true,
	"watermark":       true,
}

// importGlobals copies scalar top-level keys into the settings table.
//
// A key the schema does not know, or a value it rejects, is a warning rather
// than a failure: a config written for a newer build, or one with a typo'd
// position, should still import with that single field left at its default.
// Refusing to start would be a worse outcome than ignoring one line.
func (im *importer) importGlobals(root *yaml.Node) error {
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		value := root.Content[i+1]

		if knownSections[key] || im.isTemplateAnchor(root.Content[i], value) {
			continue
		}

		// A nested mapping becomes dotted keys, because that is how the file
		// spells a namespaced setting: `daemon:` / `  interval: 60` is the
		// setting `daemon.interval`. Skipping non-scalars here, as an earlier
		// revision did, silently dropped every such value.
		if value.Kind == yaml.MappingNode {
			if err := im.importNestedGlobals(key, value); err != nil {
				return err
			}
			continue
		}
		if value.Kind != yaml.ScalarNode {
			logrus.WithField("key", key).Debug(
				"skipping a top-level key that is neither a scalar nor a mapping",
			)
			continue
		}

		if err := im.importGlobal(key, value); err != nil {
			return err
		}
	}
	return nil
}

// importGlobal stores one scalar setting.
//
// The old format used flat names ("crop", "reference-width") while the store
// uses namespaced ones, so LookupLegacy accepts either and an existing file
// keeps importing after a rename.
//
// An unrecognised key, or a value the schema rejects, is a warning rather than
// a failure: a file written for a newer build, or one with a single typo,
// should still import with that one field left at its default. Everything
// ignored is logged with enough detail to act on.
func (im *importer) importGlobal(key string, value *yaml.Node) error {
	def, ok := config.LookupLegacy(key)
	if !ok {
		logrus.WithField("key", key).Warn(
			"ignoring unrecognised setting from the configuration file",
		)
		return nil
	}
	if def.Key != key {
		logrus.WithField("from", key).WithField("to", def.Key).Debug(
			"mapping a legacy setting name",
		)
	}

	raw := normaliseScalar(value, def.Kind)
	if err := def.ValidateValue(raw); err != nil {
		logrus.WithFields(logrus.Fields{
			"key":      def.Key,
			"value":    raw,
			"kind":     def.Kind,
			"expected": def.EnumValues,
		}).WithError(err).Warn(
			"ignoring an invalid setting from the configuration file; using the default",
		)
		return nil
	}
	if raw == def.Default {
		// Storing a value identical to the default would freeze it: a later
		// change to the default would no longer reach this user.
		return nil
	}
	if err := im.store.SetSetting(def.Key, raw); err != nil {
		return fmt.Errorf("storing setting %s: %w", def.Key, err)
	}
	return nil
}

// maxGlobalDepth bounds how far nested mappings are flattened. No setting is
// deeper than two segments, so this only exists to stop a pathological file
// from recursing indefinitely.
const maxGlobalDepth = 4

// importNestedGlobals flattens a nested mapping into dotted setting keys.
func (im *importer) importNestedGlobals(prefix string, node *yaml.Node) error {
	if strings.Count(prefix, ".") >= maxGlobalDepth {
		logrus.WithField("key", prefix).Warn(
			"ignoring a setting nested deeper than any known key",
		)
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := prefix + "." + node.Content[i].Value
		value := node.Content[i+1]

		if value.Kind == yaml.MappingNode {
			if err := im.importNestedGlobals(key, value); err != nil {
				return err
			}
			continue
		}
		if value.Kind != yaml.ScalarNode {
			logrus.WithField("key", key).Debug("skipping a non-scalar setting")
			continue
		}
		if err := im.importGlobal(key, value); err != nil {
			return err
		}
	}
	return nil
}

// normaliseScalar renders a YAML scalar as the string the store expects.
// YAML's permissive booleans ("yes", "on") are normalised so that a crop value
// of `"yes"` stays the string "yes" while a `debug: yes` becomes "true".
func normaliseScalar(n *yaml.Node, kind config.Kind) string {
	if kind != config.KindBool {
		return n.Value
	}
	var b bool
	if err := n.Decode(&b); err == nil {
		return strconv.FormatBool(b)
	}
	return n.Value
}

// isTemplateAnchor reports whether a top-level entry is a shared-settings
// block that became a channel template.
func (im *importer) isTemplateAnchor(keyNode, valueNode *yaml.Node) bool {
	if valueNode.Anchor == "" {
		return false
	}
	_, ok := im.templates[valueNode.Anchor]
	return ok
}

// importTemplates turns every anchored top-level mapping into a channel
// template. These are the "*-common-settings" blocks the example config uses
// to share an API key between channel instances.
func (im *importer) importTemplates(root *yaml.Node) error {
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		value := root.Content[i+1]

		if knownSections[key] || value.Anchor == "" || value.Kind != yaml.MappingNode {
			continue
		}

		options, err := decodeMapping(value)
		if err != nil {
			return fmt.Errorf("shared settings %q: %w", key, err)
		}

		// The type travels with the template when the block declares one, so
		// instances that only add a query keyword still know what they are.
		templateType, _ := options["type"].(string)
		delete(options, "type")

		t := store.ChannelTemplate{Name: key, Type: templateType, Options: options}
		if err := im.store.UpsertChannelTemplate(t); err != nil {
			return fmt.Errorf("storing shared settings %q: %w", key, err)
		}
		im.templates[value.Anchor] = key

		logrus.WithField("template", key).WithField(
			"anchor", value.Anchor,
		).Debug("imported shared settings as a channel template")
	}
	return nil
}

// activeChannel is one entry of the active-channels list.
type activeChannel struct {
	order       int
	probability float32
}

// importChannels builds the channel rows from the channels mapping and the
// active-channels list.
func (im *importer) importChannels(channelsNode, activeNode *yaml.Node) error {
	active := parseActiveChannels(activeNode)

	if channelsNode == nil || channelsNode.Kind != yaml.MappingNode {
		if len(active) > 0 {
			logrus.Warn("active-channels names channels but no channels section was found")
		}
		return nil
	}

	// Channels listed in active-channels keep that order. Ones defined but
	// never activated are imported disabled, so the user does not lose a
	// definition they had commented into service earlier.
	nextOrder := len(active)

	for i := 0; i+1 < len(channelsNode.Content); i += 2 {
		name := channelsNode.Content[i].Value
		node := channelsNode.Content[i+1]
		if node.Kind != yaml.MappingNode {
			logrus.WithField("channel", name).Warn("skipping a non-mapping channel definition")
			continue
		}

		c, err := im.buildChannel(name, node)
		if err != nil {
			return err
		}

		if a, ok := active[name]; ok {
			c.Enabled = true
			c.Probability = a.probability
			c.SortOrder = a.order
		} else {
			c.Enabled = false
			c.Probability = 1.0
			c.SortOrder = nextOrder
			nextOrder++
		}

		if err := im.store.UpsertChannel(c); err != nil {
			return fmt.Errorf("storing channel %q: %w", name, err)
		}
	}
	return nil
}

// buildChannel converts one channel mapping, resolving its merge key to a
// template reference and promoting the shared option columns.
func (im *importer) buildChannel(name string, node *yaml.Node) (store.ChannelInstance, error) {
	c := store.ChannelInstance{Name: name, Probability: 1.0}

	templateName, own, err := im.splitMerge(node)
	if err != nil {
		return c, fmt.Errorf("channel %q: %w", name, err)
	}
	c.TemplateName = templateName

	if t, ok := own["type"].(string); ok {
		c.Type = t
	}
	delete(own, "type")

	// Promote the three options every channel reads into their columns. Left
	// absent, they stay NULL and inherit from the template.
	if v, ok := own["strategy"]; ok {
		if s, ok := v.(string); ok {
			c.Strategy = &s
		}
		delete(own, "strategy")
	}
	if v, ok := own["width"]; ok {
		if i, ok := toInt(v); ok {
			c.Width = &i
			delete(own, "width")
		}
	}
	if v, ok := own["height"]; ok {
		if i, ok := toInt(v); ok {
			c.Height = &i
			delete(own, "height")
		}
	}

	c.Options = own

	if c.Type == "" {
		// Fall back to the template's type, then to the channel's own name,
		// matching the old behaviour where a channel key often equalled its
		// type.
		if templateName != "" {
			if t, err := im.store.ChannelTemplate(templateName); err == nil && t.Type != "" {
				c.Type = t.Type
			}
		}
		if c.Type == "" {
			return c, fmt.Errorf("channel %q has no type", name)
		}
	}
	return c, nil
}

// splitMerge separates a channel's merge-key reference from its own keys.
//
// This is the whole reason the import works on nodes: by the time yaml has
// unmarshalled a mapping, "<<: *anchor" is indistinguishable from the keys
// having been typed out longhand.
func (im *importer) splitMerge(node *yaml.Node) (templateName string, own map[string]any, err error) {
	own = map[string]any{}

	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i]
		value := node.Content[i+1]

		if key.Tag == "!!merge" || key.Value == "<<" {
			anchor := value.Anchor
			if value.Alias != nil {
				anchor = value.Alias.Anchor
			}
			if name, ok := im.templates[anchor]; ok {
				templateName = name
			} else {
				// An anchor we never saw as a top-level block: inline it
				// rather than dropping the settings on the floor.
				merged, merr := decodeMapping(value)
				if merr != nil {
					return "", nil, merr
				}
				for k, v := range merged {
					own[k] = v
				}
				logrus.WithField("anchor", anchor).Debug(
					"merge key had no matching shared block; inlining its values",
				)
			}
			continue
		}

		var decoded any
		if derr := value.Decode(&decoded); derr != nil {
			return "", nil, fmt.Errorf("option %q: %w", key.Value, derr)
		}
		own[key.Value] = decoded
	}
	return templateName, own, nil
}

// importWatermarks copies the watermark list into watermark_layers.
func (im *importer) importWatermarks(node *yaml.Node) error {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}

	for idx, item := range node.Content {
		var raw struct {
			Font             string  `yaml:"font"`
			Point            float64 `yaml:"point"`
			Color            string  `yaml:"color"`
			Position         string  `yaml:"position"`
			HMargin          float64 `yaml:"h-margin"`
			VMargin          float64 `yaml:"v-margin"`
			Linespace        float64 `yaml:"linespace"`
			Alignment        string  `yaml:"alignment"`
			Template         string  `yaml:"template"`
			AbsolutePoint    bool    `yaml:"absolute-point"`
			AbsolutePosition bool    `yaml:"absolute-position"`
			Background       struct {
				Color       string    `yaml:"color"`
				HThroughout bool      `yaml:"h-throughout"`
				VThroughout bool      `yaml:"v-throughout"`
				Paddings    []float64 `yaml:"paddings"`
			} `yaml:"background"`
		}
		if err := item.Decode(&raw); err != nil {
			return fmt.Errorf("watermark %d: %w", idx, err)
		}

		layer := store.WatermarkLayer{
			SortOrder: idx,
			Enabled:   true,
			Font:      orDefault(raw.Font, "NotoSans-Regular.ttf"),
			Point:     orDefaultFloat(raw.Point, 13),
			Color:     orDefault(raw.Color, "222222"),
			// An unrecognised position or alignment is corrected to the
			// default here, because the store rejects it outright and one
			// typo should not abort the whole import.
			Position:         validOr(raw.Position, config.ValidPosition, config.PositionBottomCenter, idx, "position"),
			HMargin:          raw.HMargin,
			VMargin:          raw.VMargin,
			Linespace:        orDefaultFloat(raw.Linespace, 1.4),
			Alignment:        validOr(raw.Alignment, config.ValidAlignment, config.AlignCenter, idx, "alignment"),
			Template:         raw.Template,
			AbsolutePoint:    raw.AbsolutePoint,
			AbsolutePosition: raw.AbsolutePosition,
			Background: store.WatermarkBackground{
				Color:       raw.Background.Color,
				HThroughout: raw.Background.HThroughout,
				VThroughout: raw.Background.VThroughout,
				Paddings:    raw.Background.Paddings,
			},
		}

		if err := im.store.UpsertWatermarkLayer(layer); err != nil {
			return fmt.Errorf("storing watermark %d: %w", idx, err)
		}
	}
	return nil
}

// validOr keeps a value if the vocabulary accepts it, else substitutes the
// default and says so.
func validOr(value string, valid func(string) bool, fallback string, idx int, field string) string {
	if value == "" {
		return fallback
	}
	if valid(value) {
		return value
	}
	logrus.WithField("watermark", idx).WithField(field, value).Warnf(
		"unrecognised %s, using %q instead", field, fallback,
	)
	return fallback
}

// parseActiveChannels reads the active-channels list, whose entries are either
// a bare name or a single-key name-to-probability mapping.
func parseActiveChannels(node *yaml.Node) map[string]activeChannel {
	active := map[string]activeChannel{}
	if node == nil || node.Kind != yaml.SequenceNode {
		return active
	}

	for idx, item := range node.Content {
		switch item.Kind {
		case yaml.ScalarNode:
			active[item.Value] = activeChannel{order: idx, probability: 1.0}
		case yaml.MappingNode:
			if len(item.Content) != 2 {
				logrus.WithField("index", idx).Warn(
					"skipping an active-channels entry with multiple keys",
				)
				continue
			}
			name := item.Content[0].Value
			var p float32
			if err := item.Content[1].Decode(&p); err != nil {
				logrus.WithField("channel", name).Warn(
					"skipping an active-channels entry with a non-numeric probability",
				)
				continue
			}
			if p < 0 || p > 1 {
				logrus.WithField("channel", name).WithField("probability", p).Warn(
					"probability is outside [0, 1], clamping",
				)
				p = clamp01(p)
			}
			active[name] = activeChannel{order: idx, probability: p}
		default:
			logrus.WithField("index", idx).Warn("skipping an unreadable active-channels entry")
		}
	}
	return active
}

// mappingValue finds a top-level key's value node.
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// decodeMapping decodes a mapping node into a plain map, resolving any merge
// keys it contains.
func decodeMapping(node *yaml.Node) (map[string]any, error) {
	target := node
	if node.Alias != nil {
		target = node.Alias
	}

	var out map[string]any
	if err := target.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	// yaml.v3 leaves the merge key in place when decoding into a map.
	delete(out, "<<")
	return out, nil
}

func toInt(v any) (int, bool) {
	switch t := v.(type) {
	case int:
		return t, true
	case int64:
		return int(t), true
	case float64:
		return int(t), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		return i, err == nil
	default:
		return 0, false
	}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func orDefaultFloat(v, fallback float64) float64 {
	if v == 0 {
		return fallback
	}
	return v
}

func clamp01(p float32) float32 {
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}
