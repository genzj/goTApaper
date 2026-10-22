package history

// History item
type History struct {
	Name    string
	History string
}

// NewHistory for an channel
func NewHistory(name string) *History {
	return &History{Name: name}
}

// Has url be recorded in history
func (h History) Has(url string) bool {
	return h.History != "" && url == h.History
}

// Mark a url to have been downloaded
func (h *History) Mark(url string) {
	h.History = url
}

// Manager loads/saves history from/to disk
type Manager interface {
	Load(name string) (*History, error)
	Save(h *History) error
}

// Default is the manager channels use. It is installed at startup, once the
// configuration store is open.
//
// A package-level default is kept because channels reach for it directly, and
// threading a manager through the Channel interface would widen that interface
// for every provider. Until one is installed, a no-op manager keeps unit tests
// of individual channels working without a database.
var Default Manager = noopManager{}

// SetDefault installs the process-wide history manager.
func SetDefault(m Manager) { Default = m }

// noopManager records nothing and reports no history, so a channel asked to
// download without a configured store simply never dedupes.
type noopManager struct{}

func (noopManager) Load(name string) (*History, error) { return NewHistory(name), nil }
func (noopManager) Save(*History) error                { return nil }
