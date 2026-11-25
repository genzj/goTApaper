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
//
// Every size and position is relative to the picture, never in pixels, so a
// layer looks the same on a 1280-pixel preview and a 5K wallpaper, and a
// preset is plain data that means the same thing on every machine:
//
//   - Size is a percentage of the picture's shorter side (CSS vmin).
//   - X and Y place the text box's Anchor point as fractions of the picture's
//     width and height.
//   - Paddings and effect sizes are in em: multiples of the font size.
type WatermarkLayer struct {
	ID        int64 `json:"id"`
	SortOrder int   `json:"sortOrder"`
	Enabled   bool  `json:"enabled"`
	// Name labels the layer in the editor. It has no effect on rendering.
	Name string `json:"name"`

	// Template is a Go text/template rendered against the picture metadata.
	// Unrelated to ChannelTemplate.
	Template string `json:"template"`

	// Font is a font file name, resolved in the platform font directories, or
	// a path.
	Font string `json:"font"`
	// Size is the font size as a percentage of the picture's shorter side.
	Size  float64 `json:"size"`
	Color string  `json:"color"`

	// Anchor is the point of the text box that X and Y place, one of
	// config.PositionValues.
	Anchor string `json:"anchor"`
	// X and Y are the anchor's position as fractions of the picture's width
	// and height: 0,0 is the top-left corner and 1,1 the bottom-right.
	X float64 `json:"x"`
	Y float64 `json:"y"`

	// Alignment is one of config.AlignmentValues.
	Alignment string  `json:"alignment"`
	Linespace float64 `json:"linespace"`

	Background WatermarkBackground `json:"background"`
	Shadow     WatermarkShadow     `json:"shadow"`
	Outline    WatermarkOutline    `json:"outline"`
}

// WatermarkBackground describes the box drawn behind a watermark's text.
type WatermarkBackground struct {
	// Color is empty when no background should be rendered.
	Color string `json:"color"`
	// HThroughout and VThroughout stretch the box across the whole picture.
	HThroughout bool `json:"hThroughout"`
	VThroughout bool `json:"vThroughout"`
	// Padding is the space around the text, in em: top, right, bottom, left.
	Padding [4]float64 `json:"padding"`
}

// WatermarkShadow is a drop shadow drawn under a watermark's text. An empty
// Color draws none.
type WatermarkShadow struct {
	Color string `json:"color"`
	// DX and DY offset the shadow, in em; positive is right and down.
	DX float64 `json:"dx"`
	DY float64 `json:"dy"`
}

// WatermarkOutline is a stroke drawn around a watermark's text. An empty
// Color or a zero Width draws none.
type WatermarkOutline struct {
	Color string `json:"color"`
	// Width is the stroke width, in em.
	Width float64 `json:"width"`
}

// WallpaperHistoryEntry is one applied wallpaper, as recorded in the history.
//
// The history is also what deduplicates downloads: a channel skips a picture
// whose URL matches its newest entry here (see LastAppliedURL).
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

	UploadTime int64 `json:"uploadTime,omitempty"`
	// DownloadTime is when the picture was last applied. A re-application
	// updates it, so an entry applied again sorts as recent.
	DownloadTime int64 `json:"downloadTime"`
	// FirstAppliedTime is when the picture was first applied. It equals
	// DownloadTime for an entry applied only once.
	FirstAppliedTime int64 `json:"firstAppliedTime"`
	// TimesApplied counts applications, re-applications included.
	TimesApplied int `json:"timesApplied"`

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
	// ReplaceWatermarkLayers replaces every layer with the given list, in
	// that order, in one transaction. A layer whose ID matches an existing
	// one keeps it; the rest get new IDs. It returns the stored list.
	ReplaceWatermarkLayers(layers []WatermarkLayer) ([]WatermarkLayer, error)

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
	// WallpaperHistoryEntryByURL returns the entry for a picture, or
	// ErrNotFound. There is at most one per URL.
	WallpaperHistoryEntryByURL(url string) (WallpaperHistoryEntry, error)
	// MarkReapplied records that an entry was applied again: it becomes the
	// newest entry, keeping when it was first applied, rather than a duplicate
	// being appended. A non-empty thumbPath replaces the entry's thumbnail,
	// for one whose file had gone. It returns ErrNotFound if the entry was
	// deleted in the meantime.
	MarkReapplied(id, at int64, filePath, thumbPath string) error
	// LastAppliedURL returns the URL of the newest entry for a channel
	// instance, or "" if it has none. It is what deduplicates downloads: a
	// channel skips a picture it applied last time.
	LastAppliedURL(channelName string) (string, error)
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
	// SetThumbnail records a thumbnail for an entry, returning the path it
	// replaces ("" if none) so the caller can remove that file. It returns
	// ErrNotFound for a missing entry.
	SetThumbnail(id int64, path string) (string, error)
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
