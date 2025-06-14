package util

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"golang.org/x/net/proxy"
)

// getHTTPClient returns http client with proper proxy settings
func getHTTPClient() (*http.Client, error) {
	var err error
	var parsed *url.URL

	var dialer proxy.Dialer

	conf := viper.GetString("proxy")

	switch strings.ToLower(conf) {
	case "direct":
		// leave for http transport
	case "environment":
		// leave for http transport
	default:
		if parsed, err = url.Parse(conf); err != nil {
			dialer = nil
		} else if parsed.Scheme == "socks5" {
			// use x/net/proxy to handle socks5 proxy
			dialer, err = proxy.FromURL(parsed, proxy.Direct)
		}
	}
	logrus.WithField("dialer", dialer).WithField("conf", conf).Debugf("dialer is of %T type", dialer)
	if err != nil {
		return nil, err
	}
	httpTransport := &http.Transport{}
	httpClient := &http.Client{Transport: httpTransport}
	if dialer != nil {
		// set socks5 as the dialer
		httpTransport.Dial = dialer.Dial
	} else if strings.ToLower(conf) == "environment" {
		httpTransport.Proxy = http.ProxyFromEnvironment
	} else if parsed != nil {
		httpTransport.Proxy = http.ProxyURL(parsed)
	}
	return httpClient, err

}

// Head sends requests with HEAD method
func Head(url string, followRedirection bool) (*http.Response, error) {
	httpClient, err := getHTTPClient()
	if err != nil {
		logrus.Error("cannot initiate http client")
		logrus.Fatal(err)
		return nil, err
	}

	if !followRedirection {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	resp, err := httpClient.Head(url)
	if err != nil {
		logrus.Errorf("http HEAD encounter error %v", err)
		return nil, err
	}
	return resp, err
}

// IsReachableLink checks reachability of URL
func IsReachableLink(url string) bool {
	response, err := Head(url, false)
	if err != nil {
		return false
	}
	logrus.Debugf("HEAD response %s", response.Status)
	return (response.StatusCode / 100) == 2
}

// DoAndExpectType sends request to server and expects response with specified
// content type
func DoAndExpectType(req *http.Request, expected string) (*http.Response, error) {
	client, err := getHTTPClient()
	if err != nil {
		logrus.Error("cannot initiate http client")
		logrus.Fatal(err)
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	logrus.WithFields(logrus.Fields{
		"url":         req.URL,
		"len":         resp.ContentLength,
		"status-code": resp.StatusCode,
		"status":      resp.Status,
	}).Debug("server responded")

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

// GetInType sends a GET request and expects a response with certain content type
func GetInType(url, expected string) (*http.Response, error) {
	logrus.Debugf("get %s", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := DoAndExpectType(req, expected)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Get a response from specified URL
func Get(url string) (*http.Response, error) {
	return GetInType(url, "")
}

// DoAndReadJSON sends request and parse its JSON response
func DoAndReadJSON(req *http.Request, obj interface{}) error {
	resp, err := DoAndExpectType(req, "application/json")
	if err != nil {
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(data, obj); err != nil {
		return err
	}

	return nil
}

// ReadJSON send a GET request to URL and parse its JSON response
func ReadJSON(url string, obj interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	return DoAndReadJSON(req, obj)
}

// Extractor is a function type that takes an io.Reader and returns extracted bytes and error
// It is used to extract JSON contents out of a response unmarshalling
type Extractor func(io.Reader) ([]byte, error)

// ExtractJSON sends a GET request to the URL, extracts data using the provided Extractor function,
// and unmarshals the extracted JSON data into obj
func ExtractJSON(url string, obj interface{}, extract Extractor) error {
	resp, err := Get(url)

	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := extract(resp.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, obj)
}

