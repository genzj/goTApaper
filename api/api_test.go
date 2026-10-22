package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config"
	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/config/store/sqlite"
	"github.com/genzj/goTApaper/service"
)

const testToken = "test-token"

func init() {
	config.SetAppName("goTApaper")
}

func newTestServer(t *testing.T) (http.Handler, store.Store) {
	t.Helper()

	s, err := sqlite.Open(sqlite.MemoryPath)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	t.Cleanup(func() { s.Close() })

	h := &handlers{
		config: service.NewConfig(s),
		wallpaper: service.NewWallpaper(s, func([]string) (*channel.PictureMeta, error) {
			return &channel.PictureMeta{ChannelKey: "bing", Title: "Test"}, nil
		}),
	}
	return h.routes(testToken), s
}

func do(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestAuthRequired is the load-bearing test of this package. The API rewrites
// configuration and holds the user's provider API keys, so an unauthenticated
// request must never reach a handler.
func TestAuthRequired(t *testing.T) {
	h, _ := newTestServer(t)

	for _, tc := range []struct {
		name  string
		token string
	}{
		{"no token", ""},
		{"wrong token", "not-the-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, "GET", "/api/v1/channels", "", tc.token)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
			if body := rec.Body.String(); strings.Contains(body, "bing") {
				t.Error("an unauthenticated response leaked configuration")
			}
		})
	}

	// A write must be refused too, not just a read.
	rec := do(t, h, "PUT", "/api/v1/settings/picture.crop", `{"value":"no"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated write status = %d, want 401", rec.Code)
	}
}

func TestHealthNeedsNoToken(t *testing.T) {
	h, _ := newTestServer(t)

	rec := do(t, h, "GET", "/api/v1/healthz", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	h, _ := newTestServer(t)

	rec := do(t, h, "PUT", "/api/v1/settings/picture.crop", `{"value":"no"}`, testToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204: %s", rec.Code, rec.Body)
	}

	rec = do(t, h, "GET", "/api/v1/settings/picture.crop", "", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	var view service.SettingView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode = %v", err)
	}
	if view.Value != config.CropNo {
		t.Errorf("value = %q, want %q", view.Value, config.CropNo)
	}
	// The schema travels with the value, so a client can render a control
	// without its own copy of the vocabulary.
	if len(view.EnumValues) == 0 {
		t.Error("the response should carry the accepted values")
	}
}

func TestSettingRejectsInvalidValue(t *testing.T) {
	h, _ := newTestServer(t)

	rec := do(t, h, "PUT", "/api/v1/settings/picture.crop", `{"value":"maybe"}`, testToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode = %v", err)
	}
	// Telling the client what *is* acceptable is the difference between a
	// usable error and a dead end.
	if len(body.Expected) == 0 {
		t.Errorf("error body should list the accepted values: %+v", body)
	}
}

// TestUnknownKeyAndUnknownPathDiffer keeps the two failure modes
// distinguishable: 400 means the route matched but the key was wrong, 404
// means there is no such endpoint. Collapsing them would leave a client unable
// to tell a typo in its own URL from a typo in the user's input.
func TestUnknownKeyAndUnknownPathDiffer(t *testing.T) {
	h, _ := newTestServer(t)

	rec := do(t, h, "GET", "/api/v1/settings/nope", "", testToken)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown setting key: status = %d, want 400", rec.Code)
	}

	rec = do(t, h, "PUT", "/api/v1/settings/nope", `{"value":"x"}`, testToken)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("writing an unknown key: status = %d, want 400", rec.Code)
	}

	rec = do(t, h, "GET", "/api/v1/not-a-real-endpoint", "", testToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown path: status = %d, want 404", rec.Code)
	}
}

// TestErrorsAreAlwaysJSON covers the catch-alls. A client decodes every
// response as JSON, so a plain-text body from the mux's built-in 404 would
// surface as a parse error rather than the actual problem.
func TestErrorsAreAlwaysJSON(t *testing.T) {
	h, _ := newTestServer(t)

	cases := []struct {
		name, method, path, token string
	}{
		{"unknown path", "GET", "/api/v1/nope", testToken},
		{"unrouted prefix", "GET", "/totally/elsewhere", testToken},
		{"unsupported method", "DELETE", "/api/v1/channel-types", testToken},
		{"unsupported method on wildcard", "POST", "/api/v1/settings/picture.crop", testToken},
		{"unauthenticated", "GET", "/api/v1/channels", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.path, "", tc.token)
			if rec.Code < 400 {
				t.Fatalf("status = %d, want an error", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var body errorBody
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON (%v): %s", err, rec.Body)
			}
			if body.Error == "" {
				t.Errorf("error body has no message: %s", rec.Body)
			}
		})
	}
}

// TestPanicBecomesJSONError checks the recovery middleware: net/http would
// otherwise drop the connection, which a client sees as an unexplained
// transport failure rather than a server error.
func TestPanicBecomesJSONError(t *testing.T) {
	panicking := recoverPanics(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	panicking.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON (%v): %s", err, rec.Body)
	}
	// The panic value can carry internals, so it must not reach the client.
	if strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("the panic value leaked to the client: %s", rec.Body)
	}
}

// TestSecretsAreRedacted checks that an API key never leaves the process.
func TestSecretsAreRedacted(t *testing.T) {
	h, s := newTestServer(t)

	const realKey = "super-secret-key"
	if err := s.UpsertChannelTemplate(store.ChannelTemplate{
		Name:    "unsplash-common",
		Type:    "unsplash",
		Options: map[string]any{"key": realKey, "strategy": config.ByWidth, "width": 1920},
	}); err != nil {
		t.Fatalf("UpsertChannelTemplate = %v", err)
	}
	if err := s.UpsertChannel(store.ChannelInstance{
		Name: "unsplash-kw-color", Type: "unsplash",
		TemplateName: "unsplash-common", Enabled: true, Probability: 1,
		SortOrder: store.AppendOrder,
		Options:   map[string]any{"query": "color"},
	}); err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}

	for _, path := range []string{
		"/api/v1/channels", "/api/v1/channel-templates",
		"/api/v1/channels/unsplash-kw-color",
	} {
		rec := do(t, h, "GET", path, "", testToken)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, rec.Code)
		}
		if strings.Contains(rec.Body.String(), realKey) {
			t.Errorf("GET %s leaked the API key:\n%s", path, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), service.Redacted) {
			t.Errorf("GET %s should show the redaction sentinel:\n%s", path, rec.Body)
		}
	}
}

// TestRedactedWriteDoesNotClobberSecret covers the round trip a form makes: it
// was shown a mask, so submitting that mask back must leave the stored key
// alone rather than overwriting it with asterisks.
func TestRedactedWriteDoesNotClobberSecret(t *testing.T) {
	h, s := newTestServer(t)

	const realKey = "super-secret-key"
	if err := s.UpsertChannel(store.ChannelInstance{
		Name: "unsplash-kw-color", Type: "unsplash", Enabled: true, Probability: 1,
		SortOrder: store.AppendOrder,
		Options: map[string]any{
			"key": realKey, "query": "color", "strategy": config.ByWidth,
		},
	}); err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}

	body := `{"name":"unsplash-kw-color","type":"unsplash","enabled":true,` +
		`"probability":1,"options":{"key":"` + service.Redacted + `","query":"water",` +
		`"strategy":"by-width"}}`
	rec := do(t, h, "PUT", "/api/v1/channels/unsplash-kw-color", body, testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200: %s", rec.Code, rec.Body)
	}

	// The edit the client actually made took effect...
	eff, err := s.EffectiveChannel("unsplash-kw-color")
	if err != nil {
		t.Fatalf("EffectiveChannel = %v", err)
	}
	if eff.Options["query"] != "water" {
		t.Errorf("query = %v, want water", eff.Options["query"])
	}
	// ...and the key it was never shown survived.
	if eff.Options["key"] != realKey {
		t.Errorf("key = %v, want the original to be preserved", eff.Options["key"])
	}
}

// TestDeleteTemplateInUseReturnsConflict checks the client gets enough to act
// on: a 409 naming the channels, so it can offer the inline action.
func TestDeleteTemplateInUseReturnsConflict(t *testing.T) {
	h, s := newTestServer(t)

	if err := s.UpsertChannelTemplate(store.ChannelTemplate{
		Name: "shared", Options: map[string]any{"key": "k"},
	}); err != nil {
		t.Fatalf("UpsertChannelTemplate = %v", err)
	}
	if err := s.UpsertChannel(store.ChannelInstance{
		Name: "dependent", Type: "unsplash", TemplateName: "shared",
		Enabled: true, Probability: 1, SortOrder: store.AppendOrder,
	}); err != nil {
		t.Fatalf("UpsertChannel = %v", err)
	}

	rec := do(t, h, "DELETE", "/api/v1/channel-templates/shared", "", testToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode = %v", err)
	}
	if len(body.UsedBy) != 1 || body.UsedBy[0] != "dependent" {
		t.Errorf("usedBy = %v, want [dependent]", body.UsedBy)
	}

	// Inlining is the way out, and must leave the channel working.
	rec = do(t, h, "POST", "/api/v1/channel-templates/shared/inline", "", testToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("inline status = %d, want 204: %s", rec.Code, rec.Body)
	}
	eff, err := s.EffectiveChannel("dependent")
	if err != nil {
		t.Fatalf("EffectiveChannel = %v", err)
	}
	if eff.Options["key"] != "k" {
		t.Errorf("key = %v; inlining must preserve the inherited value", eff.Options["key"])
	}
}

func TestChannelTypesServesSchemas(t *testing.T) {
	h, _ := newTestServer(t)

	rec := do(t, h, "GET", "/api/v1/channel-types", "", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var types []channel.TypeInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &types); err != nil {
		t.Fatalf("decode = %v", err)
	}
	if len(types) == 0 {
		t.Fatal("no channel types served")
	}

	// Each type's accepted strategies differ, which is exactly what a shared
	// enum table could not express and why the schema is served per type.
	byName := map[string]channel.OptionSchema{}
	for _, tp := range types {
		byName[tp.Name] = tp.Schema
	}
	bing, ok := byName["bing-wallpaper"].Lookup("strategy")
	if !ok {
		t.Fatal("bing-wallpaper should declare a strategy option")
	}
	if !containsStr(bing.EnumValues, config.LargestNoLogo) {
		t.Errorf("bing strategies = %v, want to include %q", bing.EnumValues, config.LargestNoLogo)
	}
	if unsplash, ok := byName["unsplash"].Lookup("strategy"); ok {
		if containsStr(unsplash.EnumValues, config.LargestNoLogo) {
			t.Error("unsplash should not offer largest-no-logo")
		}
	}
}

func TestUnknownFieldIsRejected(t *testing.T) {
	h, _ := newTestServer(t)

	// Silently ignoring a misspelled field would look like the write
	// succeeded while doing nothing.
	rec := do(t, h, "PUT", "/api/v1/settings/picture.crop", `{"valeu":"no"}`, testToken)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
}

func TestRefresh(t *testing.T) {
	h, _ := newTestServer(t)

	rec := do(t, h, "POST", "/api/v1/refresh", "", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body)
	}

	var result service.RefreshResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode = %v", err)
	}
	if result.Channel != "bing" || result.Title != "Test" {
		t.Errorf("result = %+v, want the stub's metadata", result)
	}
}

func containsStr(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// TestMethodNotAllowedIsDistinctFromNotFound covers the route table.
//
// The catch-all handlers that make every error JSON also swallow the mux's own
// 405, so without the table a client could not tell "you used the wrong verb"
// from "that URL does not exist". Both are JSON, but only one is fixable by
// changing the method.
func TestMethodNotAllowedIsDistinctFromNotFound(t *testing.T) {
	h, _ := newTestServer(t)

	cases := []struct {
		name, method, path string
		wantAllow          string
	}{
		{"static path", "DELETE", "/api/v1/channel-types", "GET"},
		{"wildcard path", "POST", "/api/v1/settings/picture.crop", "DELETE, GET, PUT"},
		{"collection", "PUT", "/api/v1/channels", "GET, POST"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.path, "", testToken)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405: %s", rec.Code, rec.Body)
			}
			// RFC 9110 requires Allow on a 405, and it is what makes the
			// response actionable.
			if got := rec.Header().Get("Allow"); got != tc.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tc.wantAllow)
			}
		})
	}

	// A genuinely unrouted path still has to be a 404, or the distinction is
	// pointless.
	rec := do(t, h, "GET", "/api/v1/no-such-collection", "", testToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unrouted path status = %d, want 404", rec.Code)
	}
	if rec.Header().Get("Allow") != "" {
		t.Error("a 404 should not carry an Allow header")
	}
}

func TestPatternMatches(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/api/v1/settings", "/api/v1/settings", true},
		{"/api/v1/settings/{key}", "/api/v1/settings/picture.crop", true},
		{"/api/v1/settings/{key}", "/api/v1/settings", false},
		// A wildcard must consume a non-empty segment, or a trailing slash
		// would masquerade as a named resource.
		{"/api/v1/settings/{key}", "/api/v1/settings/", false},
		{"/api/v1/settings/{key}", "/api/v1/settings/a/b", false},
		{"/api/v1/channel-templates/{name}/inline", "/api/v1/channel-templates/x/inline", true},
		{"/api/v1/channel-templates/{name}/inline", "/api/v1/channel-templates/x/other", false},
		{"/api/v1/settings", "/api/v1/other", false},
	}

	for _, tc := range cases {
		if got := patternMatches(tc.pattern, tc.path); got != tc.want {
			t.Errorf("patternMatches(%q, %q) = %t, want %t",
				tc.pattern, tc.path, got, tc.want)
		}
	}
}
