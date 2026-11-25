package watermark

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/genzj/goTApaper/channel"
)

// TemplateField documents one value a watermark template can show. The list is
// the single source of truth: the API serves it to the editor's completion and
// help, and a test fails if a PictureMeta field is missing from it.
type TemplateField struct {
	// Name is the field as written after the dot, e.g. "Title".
	Name string `json:"name"`
	// Type is "text" or "time".
	Type        string `json:"type"`
	Description string `json:"description"`
	// Example is a typical value, and Snippet what completion inserts.
	Example string `json:"example"`
	Snippet string `json:"snippet"`
	// Current is the field's value on the picture being edited, when there is
	// one.
	Current string `json:"current,omitempty"`
}

// TemplateFunc documents one function a watermark template can call.
type TemplateFunc struct {
	Name        string `json:"name"`
	Signature   string `json:"signature"`
	Description string `json:"description"`
	// Example is a template using the function, and Result what it renders
	// to on the example picture (exampleMeta).
	Example string `json:"example"`
	Result  string `json:"result"`
	Snippet string `json:"snippet"`
}

// TemplateFields lists every PictureMeta field, in the order the editor offers
// them.
var TemplateFields = []TemplateField{
	{
		Name: "Title", Type: "text",
		Description: "The picture's title, as the provider publishes it.",
		Example:     "Lake Bled at dawn", Snippet: "{{.Title}}",
	},
	{
		Name: "Caption", Type: "text",
		Description: "A longer description of the picture, when the provider gives one.",
		Example:     "Morning mist over the island church", Snippet: "{{.Caption}}",
	},
	{
		Name: "Credit", Type: "text",
		Description: "Who took the picture, or the copyright line the provider asks for.",
		Example:     "© Jane Doe / Example Images", Snippet: "{{.Credit}}",
	},
	{
		Name: "Channel", Type: "text",
		Description: "The kind of channel that supplied the picture (its type).",
		Example:     "bing-wallpaper", Snippet: "{{.Channel}}",
	},
	{
		Name: "ChannelKey", Type: "text",
		Description: "The name of the channel, as configured, that supplied the picture.",
		Example:     "bing", Snippet: "{{.ChannelKey}}",
	},
	{
		Name: "Format", Type: "text",
		Description: "The picture's file format.",
		Example:     "jpeg", Snippet: "{{.Format}}",
	},
	{
		Name: "URL", Type: "text",
		Description: "Where the picture was downloaded from.",
		Example:     "https://example.com/photo.jpg", Snippet: "{{.URL}}",
	},
	{
		Name: "UploadTime", Type: "time",
		Description: "When the provider published the picture. Format it with date.",
		Example:     "2026-11-24", Snippet: `{{date "2006-01-02" .UploadTime}}`,
	},
	{
		Name: "DownloadTime", Type: "time",
		Description: "When goTApaper downloaded the picture. Format it with date.",
		Example:     "2026-11-24 08:30", Snippet: `{{date "2006-01-02 15:04" .DownloadTime}}`,
	},
}

// TemplateFuncs lists the functions installed on every watermark template.
var TemplateFuncs = []TemplateFunc{
	{
		Name: "date", Signature: "date LAYOUT TIME",
		Description: `Formats a time. The layout is Go's: write how the reference time ` +
			`Mon Jan 2 15:04:05 2006 would look, e.g. "2006-01-02", "Jan 2, 2006" or "15:04". ` +
			`An unset time renders as nothing.`,
		Example: `{{date "Jan 2, 2006" .UploadTime}}`,
		Snippet: `{{date "2006-01-02" .UploadTime}}`,
	},
	{
		Name: "upper", Signature: "upper TEXT",
		Description: "Converts text to upper case.",
		Example:     "{{upper .Title}}", Snippet: "{{upper .Title}}",
	},
	{
		Name: "lower", Signature: "lower TEXT",
		Description: "Converts text to lower case.",
		Example:     "{{lower .Channel}}", Snippet: "{{lower .Channel}}",
	},
	{
		Name: "default", Signature: "default FALLBACK VALUE",
		Description: "Shows VALUE, or FALLBACK when VALUE is empty.",
		Example:     `{{default "Untitled" .Caption}}`, Snippet: `{{default "Untitled" .Title}}`,
	},
	{
		Name: "truncate", Signature: "truncate LENGTH TEXT",
		Description: "Shortens text to at most LENGTH characters, ending with … when cut.",
		Example:     "{{truncate 10 .Title}}", Snippet: "{{truncate 40 .Title}}",
	},
	{
		Name: "join", Signature: "join SEPARATOR VALUE...",
		Description: "Joins the non-empty values with SEPARATOR, so a missing one leaves no stray separator.",
		Example:     `{{join " · " .Title .Credit}}`, Snippet: `{{join " · " .Title .Credit}}`,
	},
}

// exampleMeta is the picture the documented examples render against.
var exampleMeta = channel.PictureMeta{
	Title:        "Lake Bled at dawn",
	Caption:      "",
	Credit:       "© Jane Doe / Example Images",
	Format:       "jpeg",
	Channel:      "bing-wallpaper",
	ChannelKey:   "bing",
	URL:          "https://example.com/photo.jpg",
	UploadTime:   time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC),
	DownloadTime: time.Date(2026, 11, 24, 8, 30, 0, 0, time.UTC),
}

func init() {
	for i, f := range TemplateFuncs {
		out, err := executeTemplate(f.Example, &exampleMeta)
		if err != nil {
			panic(fmt.Sprintf("watermark: example of %s does not render: %v", f.Name, err))
		}
		TemplateFuncs[i].Result = out
	}
}

// funcMap is installed on every watermark template, the refresh's and the
// preview's alike.
var funcMap = template.FuncMap{
	"date": func(layout string, t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format(layout)
	},
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"default": func(fallback string, v any) string {
		if s := toText(v); s != "" {
			return s
		}
		return fallback
	},
	"truncate": func(n int, s string) string {
		if n <= 0 || utf8.RuneCountInString(s) <= n {
			return s
		}
		r := []rune(s)
		if n == 1 {
			return "…"
		}
		return strings.TrimSpace(string(r[:n-1])) + "…"
	},
	"join": func(sep string, values ...any) string {
		parts := make([]string, 0, len(values))
		for _, v := range values {
			if s := toText(v); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, sep)
	},
}

// toText renders a template value as text, with an unset time as empty.
func toText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case time.Time:
		if x.IsZero() {
			return ""
		}
		return x.Format("2006-01-02")
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// ParseTemplate parses a watermark template with the watermark functions
// installed.
func ParseTemplate(src string) (*template.Template, error) {
	return template.New("watermark").Funcs(funcMap).Option("missingkey=error").Parse(src)
}

// CheckTemplate reports a template's parse error, if any, with its position.
func CheckTemplate(src string) *LayerError {
	if _, err := ParseTemplate(src); err != nil {
		return templateError(err)
	}
	return nil
}

// FieldsFor returns the field catalogue with each field's value on the given
// picture filled in, or without values when meta is nil.
func FieldsFor(meta *channel.PictureMeta) []TemplateField {
	out := make([]TemplateField, len(TemplateFields))
	copy(out, TemplateFields)
	if meta == nil {
		return out
	}
	for i, f := range out {
		snippet := "{{." + f.Name + "}}"
		if f.Type == "time" {
			snippet = f.Snippet
		}
		if v, err := executeTemplate(snippet, meta); err == nil {
			out[i].Current = v
		}
	}
	return out
}

// templateErrorPattern matches text/template's error prefix: the template
// name, a line and, for some errors, a column.
var templateErrorPattern = regexp.MustCompile(`^template: watermark:(\d+)(?::(\d+))?: (.*)$`)

func templateError(err error) *LayerError {
	e := &LayerError{Stage: "template", Message: err.Error()}
	if m := templateErrorPattern.FindStringSubmatch(err.Error()); m != nil {
		e.Line, _ = strconv.Atoi(m[1])
		if m[2] != "" {
			e.Column, _ = strconv.Atoi(m[2])
		}
		e.Message = m[3]
		// Execution errors repeat the location inside the message:
		// "executing \"watermark\" at <.Foo>: ...".
		e.Message = strings.TrimPrefix(e.Message, `executing "watermark" at `)
	}
	return e
}
