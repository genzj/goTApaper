package config

import (
	"path"
	"sync"

	"github.com/sirupsen/logrus"
)

// Emitter keeps configuration listening callbacks
type Emitter struct {
	l         *sync.Mutex
	listeners map[string][]func(key string, old, new interface{})
}

// NewEmitter creates a config event emitter
func NewEmitter() *Emitter {
	return &Emitter{
		l:         &sync.Mutex{},
		listeners: make(map[string][]func(key string, old, new interface{})),
	}
}

// Observe configuration updates with certain patterns
func (e *Emitter) Observe(pattern string, cb func(key string, old, new interface{})) {
	e.l.Lock()
	defer e.l.Unlock()
	if l, ok := e.listeners[pattern]; ok {
		e.listeners[pattern] = append(l, cb)
	} else {
		e.listeners[pattern] = []func(key string, old, new interface{}){cb}
	}
}

func (e *Emitter) matched(topic string) ([]string, error) {
	var err error
	acc := []string{}
	for k := range e.listeners {
		if matched, err := path.Match(topic, k); err != nil {
			return []string{}, err
		} else if matched {
			acc = append(acc, k)
		} else {
			if matched, _ := path.Match(k, topic); matched {
				acc = append(acc, k)
			}
		}
	}
	return acc, err
}

// Emit a configuration update event
func (e *Emitter) Emit(key string, old, new interface{}) {
	e.l.Lock()
	defer e.l.Unlock()

	matched, err := e.matched(key)
	if err != nil {
		logrus.Errorf("error in emitter pattern: %s", err)
		return
	}

	for _, _topic := range matched {
		for _, l := range e.listeners[_topic] {
			go l(key, old, new)
		}
	}
}

var _emitter = NewEmitter()

// Observe configuration changes by registering a call back function
func Observe(pattern string, cb func(key string, old, new interface{})) {
	_emitter.Observe(pattern, cb)
}

// Emit a configuration change event
func Emit(key string, old, new interface{}) {
	_emitter.Emit(key, old, new)
}
