package util

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/spf13/viper"

	"golang.org/x/net/proxy"
)

// getHttpClient returns http client with proper proxy settings
func getHttpClient() (*http.Client, error) {
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
			dialer, err = nil, err
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

func Head(url string, followRedirection bool) (*http.Response, error) {
	httpClient, err := getHttpClient()
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

func IsReachableLink(url string) bool {
	response, err := Head(url, false)
	if err != nil {
		return false
	}
	logrus.Debugf("HEAD response %s", response.Status)
	return (response.StatusCode / 100) == 2
}

func GetInType(url, expected string) (*http.Response, error) {
	httpClient, err := getHttpClient()
	if err != nil {
		logrus.Error("cannot initiate http client")
		logrus.Fatal(err)
		return nil, err
	}

	logrus.Debugf("get %s", url)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("%d bytes received from %s", resp.ContentLength, url)

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

	defer func() {
		_ = resp.Body.Close()
	}()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(data, obj); err != nil {
		return err
	}

	return nil
}
