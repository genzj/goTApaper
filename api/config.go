package api

import (
	"fmt"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/genzj/goTApaper/config"
	"github.com/spf13/viper"
)

func GetConfig(w rest.ResponseWriter, req *rest.Request) {
	key := req.PathParam("key")
	if !config.IsSet(key) {
		rest.Error(w, fmt.Sprintf("'%s' is not a valid key name", key), 400)
	} else if !config.IsLeaf(key) {
		rest.Error(w, fmt.Sprintf("'%s' is not a leaf field", key), 400)
	} else {
		answer := map[string]interface{}{
			key: viper.Get(key),
		}
		w.WriteJson(&answer)
	}
}

func init() {
	routes = append(routes, rest.Get("/config/#key", GetConfig))
}
