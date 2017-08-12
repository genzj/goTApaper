package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

func GetInType(url, expected string) (*http.Response, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(
		strings.ToLower(resp.Header.Get("Content-Type")),
		strings.ToLower(expected),
	) {
		return nil, fmt.Errorf(
			"Response not in type %s but %s",
			expected,
			resp.Header.Get("Content-Type"),
		)
	}
	return resp, nil
}

func ReadJson(url string, obj interface{}) error {
	resp, err := GetInType(url, "application/json")
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(data, obj); err != nil {
		return err
	}

	return nil
}
