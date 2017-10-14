package config

import "github.com/spf13/viper"

func IsSet(key string) bool {
	return viper.IsSet(key)
}

func IsLeaf(key string) bool {
	return IsSet(key) && len(viper.GetStringMap(key)) == 0
}
