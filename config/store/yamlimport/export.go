package yamlimport

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
)

// Export renders the stored configuration as YAML.
//
// Channel templates come back out as real anchors, and the channels that
// inherit them as "<<:" merge keys, so an exported file is as readable and as
// editable as a hand-written one and re-imports to the same state.
func Export(s store.Store) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}

	if err := exportSettings(s, root); err != nil {
		return nil, err
	}

	templateAnchors, err := exportTemplates(s, root)
	if err != nil {
		return nil, err
	}

	if err := exportWatermarks(s, root); err != nil {
		return nil, err
	}

	if err := exportChannels(s, root, templateAnchors); err != nil {
		return nil, err
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("cannot render configuration as YAML: %w", err)
	}
	return out, nil
}

// exportSettings writes the settings the user actually changed. Defaults are
// omitted so the file shows intent rather than a wall of unchanged values.
func exportSettings(s store.Store, root *yaml.Node) error {
	settings, err := s.Settings()
	if err != nil {
		return err
	}

	for _, setting := range settings {
		if setting.IsDefault {
			continue
		}
		def, ok := config.Lookup(setting.Key)
		if !ok {
			continue
		}

		// A namespaced key becomes a nested mapping, which is how the file
		// format spells it and what the importer reads back.
		placeNested(root, strings.Split(setting.Key, "."), scalarFor(setting.Value, def.Kind))
	}
	return nil
}

// placeNested stores a value at a path of mapping keys, creating or reusing
// the intermediate mappings so that two settings sharing a namespace end up
// under one parent rather than in two separate blocks.
func placeNested(mapping *yaml.Node, path []string, value *yaml.Node) {
	if len(path) == 1 {
		appendEntry(mapping, path[0], value)
		return
	}

	child := childMapping(mapping, path[0])
	placeNested(child, path[1:], value)
}

// childMapping returns the mapping stored at key, adding one if absent.
func childMapping(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key && mapping.Content[i+1].Kind == yaml.MappingNode {
			return mapping.Content[i+1]
		}
	}

	child := &yaml.Node{Kind: yaml.MappingNode}
	appendNode(mapping, key, child)
	return child
}

// exportTemplates writes each channel template as an anchored mapping and
// returns the anchor name chosen for each.
func exportTemplates(s store.Store, root *yaml.Node) (map[string]string, error) {
	templates, err := s.ChannelTemplates()
	if err != nil {
		return nil, err
	}

	anchors := map[string]string{}
	for _, t := range templates {
		node := &yaml.Node{Kind: yaml.MappingNode, Anchor: t.Name}
		if t.Type != "" {
			appendEntry(node, "type", stringScalar(t.Type))
		}
		if err := appendOptions(node, t.Options); err != nil {
			return nil, fmt.Errorf("template %q: %w", t.Name, err)
		}

		appendNode(root, t.Name, node)
		anchors[t.Name] = t.Name
	}
	return anchors, nil
}

// exportChannels writes the channels mapping and the active-channels list.
func exportChannels(s store.Store, root *yaml.Node, anchors map[string]string) error {
	channels, err := s.Channels()
	if err != nil {
		return err
	}

	active := &yaml.Node{Kind: yaml.SequenceNode}
	defined := &yaml.Node{Kind: yaml.MappingNode}

	for _, c := range channels {
		node := &yaml.Node{Kind: yaml.MappingNode}

		// Emit the merge key first, so local keys visibly override it.
		if c.TemplateName != "" {
			if anchor, ok := anchors[c.TemplateName]; ok {
				// The tag is left empty so the emitter writes a plain
				// "<<: *anchor" rather than an explicit "!!merge <<:". Both
				// parse, but only the former looks like something a person
				// would have typed.
				node.Content = append(node.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "<<"},
					&yaml.Node{Kind: yaml.AliasNode, Value: anchor},
				)
			}
		}

		appendEntry(node, "type", stringScalar(c.Type))
		if c.Strategy != nil {
			appendEntry(node, "strategy", stringScalar(*c.Strategy))
		}
		if c.Width != nil {
			appendEntry(node, "width", intScalar(*c.Width))
		}
		if c.Height != nil {
			appendEntry(node, "height", intScalar(*c.Height))
		}
		if err := appendOptions(node, c.Options); err != nil {
			return fmt.Errorf("channel %q: %w", c.Name, err)
		}

		appendNode(defined, c.Name, node)

		if !c.Enabled {
			continue
		}
		if c.Probability >= 1 {
			active.Content = append(active.Content, stringScalar(c.Name))
			continue
		}
		// The single-key map form carries a probability below one.
		entry := &yaml.Node{Kind: yaml.MappingNode}
		appendEntry(entry, c.Name, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!float",
			Value: strconv.FormatFloat(float64(c.Probability), 'g', -1, 32),
		})
		active.Content = append(active.Content, entry)
	}

	appendNode(root, "active-channels", active)
	appendNode(root, "channels", defined)
	return nil
}

func exportWatermarks(s store.Store, root *yaml.Node) error {
	layers, err := s.WatermarkLayers()
	if err != nil {
		return err
	}
	if len(layers) == 0 {
		return nil
	}

	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, l := range layers {
		if !l.Enabled {
			continue
		}

		node := &yaml.Node{Kind: yaml.MappingNode}
		appendEntry(node, "font", stringScalar(l.Font))
		appendEntry(node, "point", floatScalar(l.Point))
		appendEntry(node, "color", quotedScalar(l.Color))
		appendEntry(node, "position", stringScalar(l.Position))
		appendEntry(node, "h-margin", floatScalar(l.HMargin))
		appendEntry(node, "v-margin", floatScalar(l.VMargin))
		appendEntry(node, "linespace", floatScalar(l.Linespace))
		appendEntry(node, "alignment", stringScalar(l.Alignment))
		appendEntry(node, "absolute-point", boolScalar(l.AbsolutePoint))
		appendEntry(node, "absolute-position", boolScalar(l.AbsolutePosition))

		tmpl := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: l.Template}
		if l.Template != "" {
			// Literal style keeps a multi-line template readable.
			tmpl.Style = yaml.LiteralStyle
		}
		appendNode(node, "template", tmpl)

		bg := &yaml.Node{Kind: yaml.MappingNode}
		appendEntry(bg, "color", quotedScalar(l.Background.Color))
		appendEntry(bg, "h-throughout", boolScalar(l.Background.HThroughout))
		appendEntry(bg, "v-throughout", boolScalar(l.Background.VThroughout))
		paddings := &yaml.Node{Kind: yaml.SequenceNode}
		for _, p := range l.Background.Paddings {
			paddings.Content = append(paddings.Content, floatScalar(p))
		}
		appendNode(bg, "paddings", paddings)
		appendNode(node, "background", bg)

		seq.Content = append(seq.Content, node)
	}

	if len(seq.Content) > 0 {
		appendNode(root, "watermark", seq)
	}
	return nil
}

// appendOptions writes an option map in sorted key order, so successive
// exports of the same configuration are byte-identical.
func appendOptions(node *yaml.Node, options map[string]any) error {
	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		var value yaml.Node
		if err := value.Encode(options[k]); err != nil {
			return fmt.Errorf("option %q: %w", k, err)
		}
		appendNode(node, k, &value)
	}
	return nil
}

func appendEntry(mapping *yaml.Node, key string, value *yaml.Node) {
	appendNode(mapping, key, value)
}

func appendNode(mapping *yaml.Node, key string, value *yaml.Node) {
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}

func stringScalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

// quotedScalar forces quoting, needed for colour codes: an unquoted "11223344"
// would be read back as an integer.
func quotedScalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v, Style: yaml.DoubleQuotedStyle}
}

func intScalar(v int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(v)}
}

func floatScalar(v float64) *yaml.Node {
	return &yaml.Node{
		Kind: yaml.ScalarNode, Tag: "!!float",
		Value: strconv.FormatFloat(v, 'g', -1, 64),
	}
}

func boolScalar(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
}

// scalarFor renders a stored setting value with the tag its kind implies, so
// the exported document round-trips through the importer unchanged.
func scalarFor(value string, kind config.Kind) *yaml.Node {
	switch kind {
	case config.KindBool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: value}
	case config.KindInt:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value}
	case config.KindFloat:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: value}
	default:
		// Enums and free strings are quoted, so a crop mode of "no" survives
		// instead of being read back as the boolean false.
		return quotedScalar(value)
	}
}
