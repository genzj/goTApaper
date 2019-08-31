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
