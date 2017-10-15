package config

import (
	"path"
	"sync"

	"github.com/Sirupsen/logrus"
)

type emitter struct {
	*sync.Mutex
	listeners map[string][]func(key string, old, new interface{})
}

func NewEmitter() *emitter {
	return &emitter{
		Mutex:     &sync.Mutex{},
		listeners: make(map[string][]func(key string, old, new interface{})),
	}
}

func (e *emitter) Observe(pattern string, cb func(key string, old, new interface{})) {
	e.Lock()
	defer e.Unlock()
	if l, ok := e.listeners[pattern]; ok {
		e.listeners[pattern] = append(l, cb)
	} else {
		e.listeners[pattern] = []func(key string, old, new interface{}){cb}
	}
}

func (e *emitter) matched(topic string) ([]string, error) {
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

func (e *emitter) Emit(key string, old, new interface{}) {
	e.Lock()
	defer e.Unlock()

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

func Observe(pattern string, cb func(key string, old, new interface{})) {
	_emitter.Observe(pattern, cb)
}

func Emit(key string, old, new interface{}) {
	_emitter.Emit(key, old, new)
}
