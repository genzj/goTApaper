package util

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
