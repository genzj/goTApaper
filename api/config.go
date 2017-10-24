package api

import (
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/ant0ine/go-json-rest/rest"
	"github.com/genzj/goTApaper/config"
	"github.com/spf13/viper"
)

func validateKey(w rest.ResponseWriter, key string) bool {
	if !config.IsSet(key) {
		rest.Error(w, fmt.Sprintf("'%s' is not a valid key name", key), 400)
		return false
	} else if !config.IsLeaf(key) {
		rest.Error(w, fmt.Sprintf("'%s' is not a leaf field", key), 400)
		return false
	}
	return true
}

func getConfig(w rest.ResponseWriter, req *rest.Request) {
	key := req.PathParam("key")
	if validateKey(w, key) {
		v := viper.Get(key)
		logrus.Debugf("value to return %+v %T", v, v)
		answer := map[string]interface{}{
			"key":   key,
			"value": v,
		}
		w.WriteJson(&answer)
	}
}

type updateStructure struct {
	Key   string
	Value interface{}
}

func (u updateStructure) AsBool() (value bool, ok bool) {
	if value, ok = u.Value.(bool); ok {
		return value, ok
	}
	return false, false
}

func (u updateStructure) AsInt() (value int, ok bool) {
	if f64value, ok := u.Value.(float64); ok {
		return int(f64value), ok
	}
	return 0, false
}

func (u updateStructure) AsString() (value string, ok bool) {
	if value, ok = u.Value.(string); ok {
		return value, ok
	}
	return "", false
}

func (u updateStructure) AsInterfaceSlice() (value []interface{}, ok bool) {
	if value, ok = u.Value.([]interface{}); ok {
		logrus.Debugf("here 1")
		return value, ok
	}
	logrus.Debugf("here 2")
	return nil, false
}

func updateConfig(w rest.ResponseWriter, req *rest.Request) {
	updating := updateStructure{}
	if err := req.DecodeJsonPayload(&updating); err != nil {
		rest.Error(w, fmt.Sprintf("%s", err), 400)
		return
	}

	key := req.PathParam("key")
	if !validateKey(w, key) {
		return
	}
	old := viper.Get(key)

	logrus.Debugf("value to be set %+v expect type %T, JSON value type %T", updating, old, updating.Value)

	switch old.(type) {
	case bool:
		if v, ok := updating.AsBool(); ok {
			viper.Set(key, v)
			logrus.Debugf("set %s to %v", key, v)
			finishUpdate(w, updating, old)
		}
	case int:
		if v, ok := updating.AsInt(); ok {
			viper.Set(key, v)
			logrus.Debugf("set %s to %v", key, v)
			logrus.Debugf("OK=%v", ok)
			finishUpdate(w, updating, old)
		}
	case string:
		if v, ok := updating.AsString(); ok {
			viper.Set(key, v)
			logrus.Debugf("set %s to %v", key, v)
			finishUpdate(w, updating, old)
		}
	case []interface{}:
		if v, ok := updating.AsInterfaceSlice(); ok {
			viper.Set(key, v)
			logrus.Debugf("set %s to %v", key, v)
			finishUpdate(w, updating, old)
		}
	default:
		rest.Error(w, fmt.Sprintf("unsupported config field type, key %s, type %T", key, old), 500)
	}

}

func finishUpdate(w rest.ResponseWriter, updating updateStructure, old interface{}) {
	config.SaveConfig()
	config.Emit(updating.Key, old, updating.Value)
	w.WriteJson(&map[string]interface{}{
		"key":   updating.Key,
		"value": updating.Value,
	})
}

func init() {
	routes = append(
		routes,
		rest.Get("/config/#key", getConfig),
		rest.Post("/config/#key", updateConfig),
	)
}
