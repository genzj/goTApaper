package util

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"
)

// TestExtractJSONReturnsErrorOnFailedRequest reproduces the wake-from-sleep
// crash: when the network is unavailable, Get() returns a nil response and an
// error. ExtractJSON must return that error rather than dereferencing the nil
// response (which previously panicked in both extract(resp.Body) and the
// deferred resp.Body.Close()).
func TestExtractJSONReturnsErrorOnFailedRequest(t *testing.T) {
	// Start a server then immediately close it so the address refuses
	// connections, mimicking a downed network right after resume.
	srv := httptest.NewServer(nil)
	url := srv.URL
	srv.Close()

	extractCalled := false
	extract := func(r io.Reader) ([]byte, error) {
		extractCalled = true
		return io.ReadAll(r)
	}

	var obj map[string]interface{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ExtractJSON panicked instead of returning an error: %v", r)
		}
	}()

	err := ExtractJSON(url, &obj, extract)
	if err == nil {
		t.Fatal("expected an error from a failed request, got nil")
	}
	if extractCalled {
		t.Error("extractor should not be called when the request fails")
	}
}

// TestExtractJSONSuccess verifies the happy path still works: a reachable
// endpoint has its body run through the extractor and unmarshalled.
func TestExtractJSONSuccess(t *testing.T) {
	// getHTTPClient() reads "proxy" from viper; force a direct connection so
	// the test does not depend on ambient proxy configuration.
	viper.Set("proxy", "direct")
	defer viper.Set("proxy", "")

	srv := httptest.NewServer(nil)
	defer srv.Close()

	// The default handler (nil mux) returns 404 with an empty body; use an
	// extractor that ignores the body and yields valid JSON so we exercise the
	// full read -> extract -> unmarshal path without depending on server output.
	extract := func(r io.Reader) ([]byte, error) {
		_, _ = io.Copy(io.Discard, r)
		return []byte(`{"ok":true}`), nil
	}

	var obj map[string]interface{}
	if err := ExtractJSON(srv.URL, &obj, extract); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v, _ := obj["ok"].(bool); !v {
		t.Errorf("expected obj[\"ok\"]==true, got %#v", obj)
	}
}
