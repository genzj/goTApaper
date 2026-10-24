// Package store defines the persistent configuration model and the interface
// over it.
//
// The store owns every piece of user configuration that used to live in
// config.yaml. Values are persisted; the rules about what a value may be live
// in the config package's in-code schemas, which the store consults on every
// write.
package store

import (
	"errors"
)

// Errors returned by store implementations. Callers (the CLI and the REST API)
// map these onto exit codes and HTTP statuses, so they are part of the
// package's contract rather than incidental.
var (
	// ErrNotFound indicates the requested row does not exist.
	ErrNotFound = errors.New("not found")

	// ErrExists indicates a uniquely-named row already exists.
	ErrExists = errors.New("already exists")

	// ErrTemplateInUse indicates a channel template cannot be deleted because
	// channel instances still inherit from it. Deleting it anyway would
	// silently strip those channels of their inherited options, so the caller
	// must either reassign them or inline the template first.
	ErrTemplateInUse = errors.New("channel template still in use")

	// ErrSchemaTooNew indicates the database was written by a newer build than
	// this binary understands.
	ErrSchemaTooNew = errors.New("database schema is newer than this version of the application")
)

// Setting is one global configuration value together with the schema that
// describes it, so a single API response can drive a form control.
type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// IsDefault reports that no user value is stored and Value came from the
	// in-code default.
	IsDefault bool `json:"isDefault"`
}

// ChannelTemplate is a reusable block of channel options, the structural
// replacement for a YAML anchor merged in with "<<:".
type ChannelTemplate struct {
	ID int64 `json:"id"`
	// Name is the user-facing identifier, e.g. "unsplash-common-settings".
	Name string `json:"name"`
	// Type optionally restricts the template to a single channel type.
	Type string `json:"type,omitempty"`
	// Options holds the shared settings, e.g. the API key and dimensions.
	Options map[string]any `json:"options"`
}

// AppendOrder asks the store to place a new row last rather than at an
// explicit position.
//
// It is needed because zero is a legitimate sort order -- the first channel --
// so the zero value cannot double as "unset" without silently moving whatever
// the caller meant to put first.
const AppendOrder = -1

// ChannelInstance is one configured wallpaper source. It corresponds to a
// single entry under "channels:" in the old YAML, combined with that entry's
// presence and probability in "active-channels:".
type ChannelInstance struct {
	ID int64 `json:"id"`
	// Name is the config key, e.g. "bing" or "unsplash-kw-color".
	Name string `json:"name"`
	// Type is the registry name of the channel implementation, e.g.
	// "bing-wallpaper".
	Type string `json:"type"`
	// TemplateName is the channel template this instance inherits from, empty
	// if it inherits from none.
	TemplateName string `json:"templateName,omitempty"`
	Enabled      bool   `json:"enabled"`
	// Probability in [0,1] that this channel is attempted on a given refresh.
	Probability float32 `json:"probability"`
	SortOrder   int     `json:"sortOrder"`

	// Strategy, Width and Height are promoted out of Options because every
	// channel reads them. A nil pointer means "inherit from the template".
	Strategy *string `json:"strategy,omitempty"`
	Width    *int    `json:"width,omitempty"`
	Height   *int    `json:"height,omitempty"`

	// Options holds this instance's own type-specific settings. It overrides
	// the template's options key by key.
	Options map[string]any `json:"options"`

	// OptionsVersion is the channel type's option-schema version these
	// options were written against. A channel whose option shape has changed
	// can use it to recognise an older row and convert it, which is the case
	// a SQL migration cannot cover. Zero means "as written by this build".
	OptionsVersion int `json:"optionsVersion,omitempty"`
}

// EffectiveChannel is a channel instance with its template already applied:
// template options, overlaid by instance options, overlaid by the promoted
// columns. This is what the download pipeline actually reads.
type EffectiveChannel struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Enabled     bool           `json:"enabled"`
	Probability float32        `json:"probability"`
	SortOrder   int            `json:"sortOrder"`
	Options     map[string]any `json:"options"`
	// OptionsVersion is the schema version the stored options were written
	// against; see ChannelInstance.
	OptionsVersion int `json:"optionsVersion,omitempty"`
}

// WatermarkLayer is one rendered text overlay. Layers are an ordered list; a
// picture may carry several.
type WatermarkLayer struct {
	ID        int64 `json:"id"`
	SortOrder int   `json:"sortOrder"`
	Enabled   bool  `json:"enabled"`

	Font  string  `json:"font"`
	Point float64 `json:"point"`
	Color string  `json:"color"`

	// Position is one of config.PositionValues.
	Position string  `json:"position"`
	HMargin  float64 `json:"hMargin"`
	VMargin  float64 `json:"vMargin"`

	Linespace float64 `json:"linespace"`
	// Alignment is one of config.AlignmentValues.
	Alignment string `json:"alignment"`
	// Template is a Go text/template rendered against the picture metadata.
	// Unrelated to ChannelTemplate.
	Template string `json:"template"`

	AbsolutePoint    bool `json:"absolutePoint"`
	AbsolutePosition bool `json:"absolutePosition"`

	Background WatermarkBackground `json:"background"`
}

// WatermarkBackground describes the box drawn behind a watermark's text.
type WatermarkBackground struct {
	// Color is empty when no background should be rendered.
	Color       string    `json:"color"`
	HThroughout bool      `json:"hThroughout"`
	VThroughout bool      `json:"vThroughout"`
	Paddings    []float64 `json:"paddings"`
}

// HistoryEntry records the most recent download for one channel. There is at
// most one entry per channel, matching the semantics of the history.json file
// it replaces.
type HistoryEntry struct {
	ChannelName  string `json:"channelName"`
	URL          string `json:"url"`
	Title        string `json:"title,omitempty"`
	Credit       string `json:"credit,omitempty"`
	UploadTime   int64  `json:"uploadTime,omitempty"`
	DownloadTime int64  `json:"downloadTime"`
	FilePath     string `json:"filePath,omitempty"`
}

// WallpaperHistoryEntry is one applied wallpaper, as recorded in the
// browsable history.
//
// It is distinct from HistoryEntry, which is the deduplication pointer: there
// is one HistoryEntry per channel, overwritten on every download, and one
// WallpaperHistoryEntry per picture ever applied.
type WallpaperHistoryEntry struct {
	ID int64 `json:"id"`
	// ChannelName is the channel instance that supplied the picture, and
	// ChannelType the implementation behind it. Both are stored, so a row
	// stays readable after its channel is renamed or deleted.
	ChannelName string `json:"channelName"`
	ChannelType string `json:"channelType,omitempty"`
	// URL is where the picture was downloaded from, kept so it can be
	// re-downloaded and re-applied. A provider serving time-limited URLs may
	// no longer honour it.
	URL     string `json:"url,omitempty"`
	Title   string `json:"title,omitempty"`
	Caption string `json:"caption,omitempty"`
	Credit  string `json:"credit,omitempty"`
	Format  string `json:"format,omitempty"`

	UploadTime   int64 `json:"uploadTime,omitempty"`
	DownloadTime int64 `json:"downloadTime"`

	// FilePath is where the wallpaper was written. The same path recurs on
	// every row, because the wallpaper file is overwritten on each refresh;
	// only the newest row's file still holds the picture it describes.
	FilePath string `json:"filePath,omitempty"`
	// ThumbPath is a small copy of this particular picture, which is what
	// makes an older entry previewable.
	ThumbPath string `json:"thumbPath,omitempty"`
}

// Store is the persistent configuration backend.
type Store interface {
	// Settings

	// Setting returns a single setting, falling back to the in-code default
	// when the user has not set one. It returns ErrNotFound only for keys that
	// are absent from the schema entirely.
	Setting(key string) (Setting, error)
	// Settings returns every setting in the schema, stored or defaulted.
	Settings() ([]Setting, error)
	// SetSetting validates value against the key's definition and stores it.
	SetSetting(key, value string) error
	// UnsetSetting removes a stored value, reverting the key to its default.
	UnsetSetting(key string) error

	// Channel templates

	ChannelTemplates() ([]ChannelTemplate, error)
	ChannelTemplate(name string) (ChannelTemplate, error)
	UpsertChannelTemplate(t ChannelTemplate) error
	// DeleteChannelTemplate removes a template, returning ErrTemplateInUse
	// together with the dependent channel names if any still reference it.
	DeleteChannelTemplate(name string) error
	// InlineChannelTemplate copies a template's options into each dependent
	// channel and clears their reference, then deletes the template. This is
	// the non-destructive way to get rid of a template that is in use.
	InlineChannelTemplate(name string) error
	// ChannelsUsingTemplate lists the channels that inherit from a template.
	ChannelsUsingTemplate(name string) ([]string, error)

	// Channel instances

	Channels() ([]ChannelInstance, error)
	Channel(name string) (ChannelInstance, error)
	// EffectiveChannels returns enabled channels in sort order, with templates
	// already applied. This is what the refresh pipeline consumes.
	EffectiveChannels() ([]EffectiveChannel, error)
	// EffectiveChannel resolves one channel by name regardless of whether it
	// is enabled, so an explicit CLI request can still run it.
	EffectiveChannel(name string) (EffectiveChannel, error)
	UpsertChannel(c ChannelInstance) error
	DeleteChannel(name string) error
	// ReorderChannels sets sort_order to match the given name order.
	ReorderChannels(names []string) error

	// Watermark layers

	WatermarkLayers() ([]WatermarkLayer, error)
	UpsertWatermarkLayer(l WatermarkLayer) error
	DeleteWatermarkLayer(id int64) error

	// History

	// History returns the recorded download for one channel, or ErrNotFound.
	History(channelName string) (HistoryEntry, error)
	Histories() ([]HistoryEntry, error)
	// PutHistory inserts or overwrites the entry for a channel.
	PutHistory(e HistoryEntry) error

	// Wallpaper history

	// AppendWallpaperHistory records one applied wallpaper and returns its id.
	AppendWallpaperHistory(e WallpaperHistoryEntry) (int64, error)
	// WallpaperHistory returns applied wallpapers newest first. A limit of
	// zero or less returns every row.
	WallpaperHistory(limit, offset int) ([]WallpaperHistoryEntry, error)
	// WallpaperHistoryCount reports the total number of rows, so a client can
	// page without walking them.
	WallpaperHistoryCount() (int, error)
	// WallpaperHistoryEntryByID returns one row, or ErrNotFound.
	WallpaperHistoryEntryByID(id int64) (WallpaperHistoryEntry, error)
	// PruneWallpaperHistory deletes the oldest rows beyond keepAtMost and
	// returns the thumbnail path of every row it deleted. A keepAtMost of
	// zero or less keeps everything.
	//
	// It returns paths rather than deleting the files itself so the store
	// stays free of filesystem side effects: that is what lets it be tested
	// against an in-memory database, and it lets the caller unlink only after
	// the transaction has committed.
	PruneWallpaperHistory(keepAtMost int) ([]string, error)
	// WallpaperHistoryBeyond returns the thumbnail paths a prune to keepAtMost
	// would delete, without deleting anything, so a dry run can report what it
	// would do.
	WallpaperHistoryBeyond(keepAtMost int) ([]string, error)
	// ThumbnailPaths returns every thumbnail path still referenced, for
	// reclaiming files that no row points at.
	ThumbnailPaths() ([]string, error)
	// DeleteWallpaperHistoryEntry removes one row, returning its thumbnail
	// path.
	DeleteWallpaperHistoryEntry(id int64) (string, error)

	// Lifecycle

	// Tx runs fn inside a single transaction, rolling back on error. Used by
	// the YAML import so a partial config is never persisted.
	Tx(fn func(Store) error) error
	Close() error
}
