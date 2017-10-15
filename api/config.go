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
			key: v,
		}
		w.WriteJson(&answer)
	}
}

type updateStructure struct {
	Value interface{}
}

type updateStructureBool struct {
	Value bool
}

type updateStructureInt struct {
	Value int
}

type updateStructureString struct {
	Value string
}

type updateStructureStringSlice struct {
	Value []string
}

func readUpdateValue(w rest.ResponseWriter, req *rest.Request, v interface{}) (ok bool) {
	if err := req.DecodeJsonPayload(&v); err != nil {
		rest.Error(w, fmt.Sprintf("%s", err), 400)
		ok = false
	} else {
		ok = true
	}
	return ok
}

func updateConfig(w rest.ResponseWriter, req *rest.Request) {
	var old, new interface{}
	ok := false
	key := req.PathParam("key")
	if !validateKey(w, key) {
		return
	}

	old = viper.Get(key)

	switch old.(type) {
	case bool:
		v := updateStructureBool{}
		if readUpdateValue(w, req, &v) {
			logrus.Debugf("value to be set %+v %T", v, v.Value)
			viper.Set(key, v.Value)
			new = interface{}(v.Value)
			ok = true
		}
	case int:
		v := updateStructureInt{}
		if readUpdateValue(w, req, &v) {
			logrus.Debugf("value to be set %+v %T", v, v.Value)
			viper.Set(key, v.Value)
			new = interface{}(v.Value)
			ok = true
		}
	case string:
		v := updateStructureString{}
		if readUpdateValue(w, req, &v) {
			logrus.Debugf("value to be set %+v %T", v, v.Value)
			viper.Set(key, v.Value)
			new = interface{}(v.Value)
			ok = true
		}
	case []string:
		v := updateStructureStringSlice{}
		if readUpdateValue(w, req, &v) {
			logrus.Debugf("value to be set %+v %T", v, v.Value)
			viper.Set(key, v.Value)
			new = interface{}(v.Value)
			ok = true
		}
	default:
		rest.Error(w, fmt.Sprintf("unsupported config field type, key %s, type %T", key, viper.Get(key)), 500)
	}

	if ok {
		config.SaveConfig()
		config.Emit(key, old, new)
	}
}

func init() {
	routes = append(
		routes,
		rest.Get("/config/#key", getConfig),
		rest.Post("/config/#key", updateConfig),
	)
}
