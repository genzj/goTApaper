package api

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

func TestLoadOrCreateTokenIsStable(t *testing.T) {
	dir := t.TempDir()

	first, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("first call = %v", err)
	}
	if first == "" {
		t.Fatal("generated an empty token")
	}

	// A restart must adopt the existing token, or every client's stored
	// credential would be invalidated on each daemon start.
	second, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("second call = %v", err)
	}
	if second != first {
		t.Errorf("token changed across calls: %q then %q", first, second)
	}
}

// TestLoadOrCreateTokenConcurrent documents what concurrent starts guarantee,
// which is deliberately not convergence.
//
// Two daemons starting together both generate a token and the last write wins,
// so they can return different values. That is the accepted trade for a simple
// implementation. What must still hold is that nobody errors, nobody gets an
// empty token, and the file is left holding a well-formed one rather than
// something partial.
func TestLoadOrCreateTokenConcurrent(t *testing.T) {
	dir := t.TempDir()

	// Deliberately more racers than a real machine would ever start, to give
	// any interleaving a chance to show up.
	const racers = 24
	tokens := make([]string, racers)
	errs := make([]error, racers)

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			tokens[i], errs[i] = loadOrCreateToken(dir)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("racer %d = %v", i, err)
		}
		if tokens[i] == "" {
			t.Fatalf("racer %d got an empty token", i)
		}
		assertWellFormedToken(t, tokens[i])
	}

	// The file must hold a usable token, whichever racer wrote last. Which one
	// it is cannot be asserted: that is precisely the accepted race.
	stored, err := readToken(filepath.Join(dir, "api-token"))
	if err != nil {
		t.Fatalf("readToken = %v", err)
	}
	assertWellFormedToken(t, stored)
}

// assertWellFormedToken checks a token is the full-length hex string the
// generator produces, so a truncated or partial value is caught.
func assertWellFormedToken(t *testing.T, token string) {
	t.Helper()

	if len(token) != hex.EncodedLen(tokenBytes) {
		t.Errorf("token %q is %d characters, want %d",
			token, len(token), hex.EncodedLen(tokenBytes))
		return
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Errorf("token %q is not hex: %v", token, err)
	}
}

// TestEmptyTokenFileIsReplaced covers a file left empty by an interrupted
// write, which is treated as absent rather than served as a credential.
func TestEmptyTokenFileIsReplaced(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api-token")

	if err := os.WriteFile(path, []byte("   \n"), 0600); err != nil {
		t.Fatal(err)
	}

	token, err := loadOrCreateToken(dir)
	if err != nil {
		t.Fatalf("loadOrCreateToken = %v", err)
	}
	if token == "" {
		t.Error("an empty token file should have been replaced with a real token")
	}
}

// TestUnauthorizedRequestIsLogged checks the warning exists and that it does
// not carry the presented credential.
//
// This is the only signal a user gets that something on their machine is
// probing an interface which can read their API keys, so it is logged at
// warning rather than debug.
func TestUnauthorizedRequestIsLogged(t *testing.T) {
	hook := logtest.NewLocal(logrus.StandardLogger())
	defer hook.Reset()

	previous := logrus.GetLevel()
	logrus.SetLevel(logrus.WarnLevel)
	defer logrus.SetLevel(previous)

	h, _ := newTestServer(t)
	do(t, h, "GET", "/api/v1/channels", "", "wrong-token-value")

	var found *logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Level == logrus.WarnLevel && e.Message == "rejected an unauthorized API request" {
			found = e
			break
		}
	}
	if found == nil {
		t.Fatal("no warning was logged for an unauthorized request")
	}
	if got := found.Data["credential"]; got != "incorrect" {
		t.Errorf("credential field = %v, want \"incorrect\"", got)
	}

	// The token the caller presented must never reach the log.
	if rendered, _ := found.String(); strings.Contains(rendered, "wrong-token-value") {
		t.Error("the presented token was logged")
	}
	for _, v := range found.Data {
		if s, ok := v.(string); ok && s == "wrong-token-value" {
			t.Error("the presented token was logged in a field")
		}
	}

	// A missing credential is distinguishable from a wrong one, which separates
	// a misconfigured client from a probe.
	hook.Reset()
	do(t, h, "GET", "/api/v1/channels", "", "")
	for _, e := range hook.AllEntries() {
		if e.Message == "rejected an unauthorized API request" {
			if got := e.Data["credential"]; got != "missing" {
				t.Errorf("credential field = %v, want \"missing\"", got)
			}
			return
		}
	}
	t.Error("no warning was logged for a request with no credential")
}
