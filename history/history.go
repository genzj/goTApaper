package history

type History struct {
	Name    string
	History string
}

func NewHistory(name string) *History {
	return &History{Name: name}
}

func (h History) Has(url string) bool {
	return h.History != "" && url == h.History
}

func (h *History) Mark(url string) {
	h.History = url
}

type HistoryManager interface {
	Load(name string) (*History, error)
	Save(h *History) error
}
