package api

import (
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/ant0ine/go-json-rest/rest"
)

var routes = []*rest.Route{}

func StartApiServer() {
	api := rest.NewApi()
	api.Use(rest.DefaultDevStack...)
	router, err := rest.MakeRouter(routes...)
	if err != nil {
		logrus.Error("cannot start api server")
		logrus.Fatal(err)
	}
	api.SetApp(router)
	go func() {
		logrus.Fatal(http.ListenAndServe("127.0.0.1:9073", api.MakeHandler()))
	}()
}

func init() {
	rest.ErrorFieldName = "msg"
}
