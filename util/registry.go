package util

type RegistryMap map[string]interface{}

func (m *RegistryMap) Register(name string, v interface{}) {
	(*m)[name] = v
}

func (m RegistryMap) Get(name string) (v interface{}, ok bool) {
	v, ok = (m)[name]
	return
}
