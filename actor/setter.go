package actor

import (
	"github.com/genzj/goTApaper/util"
)

// Setter set OS desktop to use downloaded wallpaper
type Setter interface {
	Set(filename string) error
}

// Setters singleton for convenience
var Setters = util.RegistryMap{}
