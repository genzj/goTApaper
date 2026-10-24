package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestSessionHandlerExchangesATicketForACookie(t *testing.T) {
	tickets := newTicketStore()
	ticket, _ := tickets.Mint()
	h := &sessionHandler{tickets: tickets, token: testToken}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, uiSessionPath+"?ticket="+ticket, nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	// Redirected to a clean URL, so the ticket does not linger in the address
	// bar or in history.
	if got := rec.Header().Get("Location"); got != uiPath {
		t.Errorf("Location = %q, want %q", got, uiPath)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("set %d cookies, want 1", len(cookies))
	}
	c := cookies[0]
	if c.Name != SessionCookieName {
		t.Errorf("cookie name = %q", c.Name)
	}
	if c.Value != testToken {
		t.Errorf("cookie does not carry the token")
	}
	if !c.HttpOnly {
		t.Error("the session cookie is not HttpOnly, so a scripting bug could read the token")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Error("the session cookie is not SameSite=Strict")
	}
}

func TestSessionHandlerRejectsAMissingOrSpentTicket(t *testing.T) {
	tickets := newTicketStore()
	ticket, _ := tickets.Mint()
	h := &sessionHandler{tickets: tickets, token: testToken}

	for _, query := range []string{"", "?ticket=", "?ticket=wrong"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, uiSessionPath+query, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%q: status = %d, want 403", query, rec.Code)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%q: a cookie was set without a valid ticket", query)
		}
	}

	// Spend it, then try again.
	ok := httptest.NewRecorder()
	h.ServeHTTP(ok, httptest.NewRequest(http.MethodGet, uiSessionPath+"?ticket="+ticket, nil))
	replay := httptest.NewRecorder()
	h.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, uiSessionPath+"?ticket="+ticket, nil))
	if replay.Code != http.StatusForbidden {
		t.Errorf("replayed ticket: status = %d, want 403", replay.Code)
	}
}

func TestSessionCookieAuthenticatesTheAPI(t *testing.T) {
	h, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: testToken})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	// A browser cannot set an Authorization header on a navigation, so the
	// cookie has to work as an equivalent credential.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestAWrongSessionCookieIsRejected(t *testing.T) {
	h, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "not-the-token"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAnExplicitBearerTokenWinsOverAStaleCookie(t *testing.T) {
	h, _ := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "stale"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: a stale cookie shadowed a valid header", rec.Code)
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

func TestTrimTicketKeepsCredentialsOutOfTheLog(t *testing.T) {
	got := trimTicket("/ui/session?ticket=super-secret-value")
	if strings.Contains(got, "super-secret-value") {
		t.Fatalf("trimTicket leaked the ticket: %q", got)
	}
	if got := trimTicket("/api/v1/settings"); got != "/api/v1/settings" {
		t.Fatalf("trimTicket altered a URL with no ticket: %q", got)
	}
}
