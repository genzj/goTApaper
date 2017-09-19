package actor

import (
	"github.com/genzj/goTApaper/util"
)

type Setter interface {
	Set(filename string) error
}

var Setters = util.RegistryMap{}
