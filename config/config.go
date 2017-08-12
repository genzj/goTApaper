package config

type ChannelConfig struct {
	Name     string
	Offset   int
	Strategy SizeStrategy
	Width    int
}

type Config struct {
	LeastRetainMinutes int
	Channels           []ChannelConfig
}
