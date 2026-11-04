package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestTicketIsSingleUse(t *testing.T) {
	tickets := newTicketStore()

	ticket, err := tickets.Mint()
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	if !tickets.Redeem(ticket) {
		t.Fatal("a fresh ticket was rejected")
	}
	// The whole point: the URL that ends up in a shell history or a crash log
	// is already worthless.
	if tickets.Redeem(ticket) {
		t.Fatal("a ticket was accepted twice")
	}
}

func TestTicketExpires(t *testing.T) {
	tickets := newTicketStore()

	now := time.Now()
	tickets.now = func() time.Time { return now }

	ticket, err := tickets.Mint()
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	now = now.Add(ticketTTL + time.Second)
	if tickets.Redeem(ticket) {
		t.Fatal("an expired ticket was accepted")
	}
}

func TestTicketRejectsUnknownAndEmptyValues(t *testing.T) {
	tickets := newTicketStore()
	if _, err := tickets.Mint(); err != nil {
		t.Fatalf("Mint: %v", err)
	}

	if tickets.Redeem("") {
		t.Error("an empty ticket was accepted")
	}
	if tickets.Redeem("not-a-ticket") {
		t.Error("an unknown ticket was accepted")
	}
}

func TestTicketsAreDistinct(t *testing.T) {
	tickets := newTicketStore()

	seen := map[string]bool{}
	for range 50 {
		ticket, err := tickets.Mint()
		if err != nil {
			t.Fatalf("Mint: %v", err)
		}
		if seen[ticket] {
			t.Fatal("Mint returned a duplicate ticket")
		}
		seen[ticket] = true
	}
}

func TestExpiredTicketsAreNotRetainedForever(t *testing.T) {
	tickets := newTicketStore()
	now := time.Now()
	tickets.now = func() time.Time { return now }

	for range 10 {
		if _, err := tickets.Mint(); err != nil {
			t.Fatalf("Mint: %v", err)
		}
	}
	now = now.Add(ticketTTL + time.Second)

	// Minting again sweeps the expired ones, so an application left running
	// does not accumulate them.
	if _, err := tickets.Mint(); err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if got := len(tickets.tickets); got != 1 {
		t.Fatalf("retained %d tickets, want 1", got)
	}
}

func exchange(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, ticketExchangePath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTicketExchangeReturnsTheTokenOnce(t *testing.T) {
	handlers, _ := newTestHandlers(t, nil)
	tickets := newTicketStore()
	h := testRoutes(handlers, tickets)
	ticket, _ := tickets.Mint()

	// No Authorization header: this is how the interface gets one.
	rec := exchange(t, h, `{"ticket":"`+ticket+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body ticketExchangeBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Token != testToken {
		t.Fatalf("token = %q, want the API token", body.Token)
	}
	// The response carries a credential, so nothing may keep a copy.
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}

	// Single-use: the URL the browser was launched with is now worthless.
	if rec := exchange(t, h, `{"ticket":"`+ticket+`"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("replayed ticket: status = %d, want 403", rec.Code)
	}
}

func TestTicketExchangeRejectsAnythingButAValidTicket(t *testing.T) {
	h, _ := newTestServer(t)

	for _, body := range []string{`{}`, `{"ticket":""}`, `{"ticket":"guess"}`} {
		rec := exchange(t, h, body)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", body, rec.Code)
		}
		if strings.Contains(rec.Body.String(), testToken) {
			t.Errorf("%s: the token leaked in a refusal", body)
		}
	}
}

func TestSessionURLCarriesTheTicketInTheFragment(t *testing.T) {
	srv := &Server{tickets: newTicketStore(), listener: fakeListener{}}

	url, err := srv.SessionURL()
	if err != nil {
		t.Fatalf("SessionURL: %v", err)
	}
	// A fragment is never sent to the server and is stripped from Referer,
	// which is the point: the ticket stays out of request logs.
	prefix := "http://127.0.0.1:8080/ui/#ticket="
	if !strings.HasPrefix(url, prefix) {
		t.Fatalf("SessionURL = %q, want the ticket after %q", url, prefix)
	}
	if !srv.tickets.Redeem(strings.TrimPrefix(url, prefix)) {
		t.Fatal("the ticket in the URL is not redeemable")
	}
}

func TestInterfaceAssetsAreServedWithoutAToken(t *testing.T) {
	SetUIAssets(fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>t</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	})
	t.Cleanup(func() { SetUIAssets(nil) })
	h, _ := newTestServer(t)

	// The page has to load before it can ask for the token. It holds no
	// secrets: everything it shows comes from API calls that carry the token.
	for _, path := range []string{"/ui/", "/ui/assets/app.js"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, rec.Code)
		}
	}
}

func TestAPIStillRequiresTheToken(t *testing.T) {
	h, _ := newTestServer(t)

	// Opening up the assets must not have opened up anything else.
	for _, path := range []string{"/api/v1/settings", "/api/v1/status", "/api/v1/events"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: status = %d, want 401", path, rec.Code)
		}
	}
}

func TestACookieIsNotACredential(t *testing.T) {
	h, _ := newTestServer(t)

	// The interface sends the token as a bearer header. A cookie would be
	// sent by the browser on its own, which is what CSRF exploits.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.AddCookie(&http.Cookie{Name: "gotapaper_session", Value: testToken})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequestsForAnotherHostAreRejected(t *testing.T) {
	handlers, _ := newTestHandlers(t, nil)
	// The raw routes, without the test shim that addresses requests to
	// loopback.
	h := handlers.routes(testToken, newTicketStore(), nil)

	// A DNS-rebinding page reaches 127.0.0.1 under the attacker's name. Even
	// the open routes must refuse it, or it could read the assets and probe
	// the ticket exchange as if it were this application.
	for _, path := range []string{"/ui/", "/api/v1/healthz", "/api/v1/settings"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Host = "rebind.attacker.example:8080"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s for a foreign host: status = %d, want 403", path, rec.Code)
		}
	}

	for _, host := range []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("healthz for %s: status = %d, want 200", host, rec.Code)
		}
	}
}

func TestCrossOriginWritesAreRejected(t *testing.T) {
	h, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/log.debug",
		strings.NewReader(`{"value":"true"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestSameOriginAndToollessWritesAreAccepted(t *testing.T) {
	for _, origin := range []string{
		// Absent, as curl and the CLI send it.
		"",
		"http://127.0.0.1:8080",
		"http://localhost:8080",
	} {
		h, _ := newTestServer(t)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/log.debug",
			strings.NewReader(`{"value":"true"}`))
		req.Header.Set("Authorization", "Bearer "+testToken)
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("Origin %q: status = %d, want 204: %s",
				origin, rec.Code, rec.Body.String())
		}
	}
}

func TestCrossOriginReadsAreStillAllowed(t *testing.T) {
	h, _ := newTestServer(t)

	// A read cannot change state, and rejecting one would break nothing an
	// attacker needs while complicating legitimate clients.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Origin", "https://evil.example")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestTicketExchangeRefusesACrossOriginPost(t *testing.T) {
	handlers, _ := newTestHandlers(t, nil)
	tickets := newTicketStore()
	h := testRoutes(handlers, tickets)
	ticket, _ := tickets.Mint()

	req := httptest.NewRequest(http.MethodPost, ticketExchangePath,
		strings.NewReader(`{"ticket":"`+ticket+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

// fakeListener reports a fixed address, for building URLs without binding.
type fakeListener struct{ net.Listener }

func (fakeListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(LoopbackHost), Port: 8080}
}

func TestIsDirectLocalRequest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remote string
		header string
		want   bool
	}{
		{"IPv4 loopback", "127.0.0.1:5000", "", true},
		{"the whole 127/8 block", "127.0.0.2:5000", "", true},
		{"IPv6 loopback", "[::1]:5000", "", true},
		{"another machine", "192.0.2.1:5000", "", false},
		{"forwarded by a local proxy", "127.0.0.1:5000", "X-Forwarded-For", false},
		{"unparseable", "nonsense", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			if tc.header != "" {
				r.Header.Set(tc.header, "203.0.113.7")
			}
			if got := isDirectLocalRequest(r); got != tc.want {
				t.Errorf("isDirectLocalRequest = %v, want %v", got, tc.want)
			}
		})
	}
}
