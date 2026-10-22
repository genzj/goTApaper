package util

import "sort"

// RegistryMap keeps any registrable pairs
type RegistryMap map[string]interface{}

// Register an item
func (m *RegistryMap) Register(name string, v interface{}) {
	(*m)[name] = v
}

// Get a registered item
func (m RegistryMap) Get(name string) (v interface{}, ok bool) {
	v, ok = m[name]
	return v, ok
}

// Names lists every registered name in sorted order. Registration happens in
// init() functions and may be build-tag dependent, so this is the only honest
// source of "what is actually available in this binary".
func (m RegistryMap) Names() []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
