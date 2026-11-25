package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/genzj/goTApaper/config/store"
	"github.com/genzj/goTApaper/service"
)

func newWatermarkTestServer(t *testing.T) (http.Handler, *handlers, store.Store) {
	t.Helper()
	h, s := newTestHandlers(t, nil)
	h.watermarks = service.NewWatermarks(s)
	return testRoutes(h, newTicketStore()), h, s
}

func TestWatermarkEditorRoutesNeedTheToken(t *testing.T) {
	routes, _, _ := newWatermarkTestServer(t)
	for _, rt := range []struct{ method, path string }{
		{"GET", "/api/v1/fonts"},
		{"GET", "/api/v1/fonts/abc/file"},
		{"GET", "/api/v1/watermark-presets"},
		{"GET", "/api/v1/watermark/template-fields"},
		{"POST", "/api/v1/watermark/preview"},
		{"GET", "/api/v1/watermark/preview/image"},
		{"POST", "/api/v1/watermark/preview/desktop"},
		{"POST", "/api/v1/watermark/preview/revert"},
		{"PUT", "/api/v1/watermark-layers"},
	} {
		if rec := do(t, routes, rt.method, rt.path, "", ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", rt.method, rt.path, rec.Code)
		}
	}
}

func TestReplaceWatermarkLayersRoute(t *testing.T) {
	routes, _, _ := newWatermarkTestServer(t)

	l := store.NewWatermarkLayer("Go-Regular.ttf")
	body, _ := json.Marshal([]store.WatermarkLayer{l, l})
	rec := do(t, routes, "PUT", "/api/v1/watermark-layers", string(body), testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body)
	}
	var saved []store.WatermarkLayer
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil || len(saved) != 2 {
		t.Fatalf("saved = %v, %v", saved, err)
	}

	// A template that does not parse is refused, naming the problem.
	l.Template = "{{.Title"
	body, _ = json.Marshal([]store.WatermarkLayer{l})
	if rec := do(t, routes, "PUT", "/api/v1/watermark-layers", string(body), testToken); rec.Code != http.StatusBadRequest {
		t.Errorf("bad template = %d, want 400: %s", rec.Code, rec.Body)
	}

	// An empty list clears the layers.
	if rec := do(t, routes, "PUT", "/api/v1/watermark-layers", "[]", testToken); rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Errorf("clearing = %d %q", rec.Code, rec.Body)
	}
}

func TestWatermarkPresetsAndTemplateFields(t *testing.T) {
	routes, _, _ := newWatermarkTestServer(t)

	rec := do(t, routes, "GET", "/api/v1/watermark-presets", "", testToken)
	var presets []struct {
		ID     string                 `json:"id"`
		Layers []store.WatermarkLayer `json:"layers"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &presets) != nil || len(presets) == 0 {
		t.Fatalf("presets = %d %s", rec.Code, rec.Body)
	}

	rec = do(t, routes, "GET", "/api/v1/watermark/template-fields", "", testToken)
	var help service.TemplateHelp
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &help) != nil ||
		len(help.Fields) == 0 || len(help.Funcs) == 0 {
		t.Fatalf("template fields = %d %s", rec.Code, rec.Body)
	}
}

func TestPreviewWithoutAWallpaperIsAConflict(t *testing.T) {
	routes, _, _ := newWatermarkTestServer(t)

	rec := do(t, routes, "POST", "/api/v1/watermark/preview", `{"layers":[]}`, testToken)
	if rec.Code != http.StatusConflict {
		t.Errorf("preview with nothing applied = %d, want 409: %s", rec.Code, rec.Body)
	}
	if rec := do(t, routes, "POST", "/api/v1/watermark/preview/desktop", "", testToken); rec.Code != http.StatusNotFound {
		t.Errorf("desktop preview before any render = %d, want 404: %s", rec.Code, rec.Body)
	}
}

func TestWatermarkEditorUnavailableWithoutTheService(t *testing.T) {
	routes, _ := newTestServer(t)
	if rec := do(t, routes, "GET", "/api/v1/fonts", "", testToken); rec.Code != http.StatusNotImplemented {
		t.Errorf("fonts without the editor = %d, want 501", rec.Code)
	}
}
