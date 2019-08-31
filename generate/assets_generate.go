// +build ignore

package main

import (
	"log"
	"net/http"

	"github.com/genzj/goTApaper/data"
	"github.com/shurcooL/vfsgen"
)

func addPathPrefix(fs http.FileSystem) http.FileSystem {
	return http.Dir("../" + (fs.(http.Dir)))
}

func main() {
	err := vfsgen.Generate(addPathPrefix(data.ExampleAssets), vfsgen.Options{
		PackageName:  "data",
		BuildTags:    "!dev",
		VariableName: "ExampleAssets",
		Filename:     "../data/example_vfsdata.go",
	})
	if err != nil {
		log.Fatalln(err)
	}
}
