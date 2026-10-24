package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/genzj/goTApaper/channel"
	"github.com/genzj/goTApaper/config/store/sqlite"
	"github.com/genzj/goTApaper/service"
)

// TestStartBindsLoopbackOnly is the most important test in this package.
//
// The API rewrites the user's configuration, can be driven to make outbound
// requests, and holds provider API keys. If it ever bound a wildcard address
// it would be remotely reachable, so the listening address is asserted rather
// than assumed.
func TestStartBindsLoopbackOnly(t *testing.T) {
	srv, _ := startRealServer(t)

	host, port, err := net.SplitHostPort(srv.Addr())
	if err != nil {
		t.Fatalf("SplitHostPort(%q) = %v", srv.Addr(), err)
	}
	if host != LoopbackHost {
		t.Fatalf("listening on %q, want %q; a wildcard bind would be remotely reachable",
			host, LoopbackHost)
	}

	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		t.Fatalf("address %q is not a loopback address", host)
	}
	if port == "0" {
		t.Error("an ephemeral port should have been resolved to a real one")
	}
}

func TestTokenFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows has no Unix permission bits; os.Stat synthesises 0666 (or
		// 0444 for a read-only file) regardless of the mode passed to
		// OpenFile, so there is nothing here to assert. Access control on
		// Windows comes from the ACL the file inherits from its directory.
		t.Skip("permission bits are not modelled on Windows")
	}

	_, dir := startRealServer(t)

	info, err := os.Stat(TokenPath(dir))
	if err != nil {
		t.Fatalf("Stat = %v", err)
	}
	// The token is the only thing standing between a local process and the
	// user's API keys, so it must not be group or world readable.
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("token file mode = %04o, want 0600", perm)
	}
}

func TestEndpointFileWrittenAndRemoved(t *testing.T) {
	srv, dir := startRealServer(t)

	recorded, err := os.ReadFile(EndpointPath(dir))
	if err != nil {
		t.Fatalf("ReadFile = %v", err)
	}
	if strings.TrimSpace(string(recorded)) != srv.Addr() {
		t.Errorf("endpoint file = %q, want %q", strings.TrimSpace(string(recorded)), srv.Addr())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown = %v", err)
	}

	// A stale address would send the CLI to a dead port.
	if _, err := os.Stat(EndpointPath(dir)); !os.IsNotExist(err) {
		t.Error("the endpoint file should be removed on shutdown")
	}
}

// TestLiveRequestRequiresToken exercises the real server over a real socket,
// rather than through httptest, to confirm the middleware is actually wired
// into the listening handler.
func TestLiveRequestRequiresToken(t *testing.T) {
	srv, dir := startRealServer(t)

	token, err := os.ReadFile(TokenPath(dir))
	if err != nil {
		t.Fatalf("ReadFile = %v", err)
	}
	base := "http://" + srv.Addr()

	// Unauthenticated.
	resp, err := http.Get(base + "/api/v1/channels")
	if err != nil {
		t.Fatalf("GET = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated status = %d, want 401", resp.StatusCode)
	}

	// Authenticated.
	req, _ := http.NewRequest("GET", base+"/api/v1/channels", nil)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	authed, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("authenticated GET = %v", err)
	}
	defer authed.Body.Close()
	if authed.StatusCode != http.StatusOK {
		t.Fatalf("authenticated status = %d, want 200", authed.StatusCode)
	}

	body, _ := io.ReadAll(authed.Body)
	var channels []service.ChannelView
	if err := json.Unmarshal(body, &channels); err != nil {
		t.Fatalf("decode = %v: %s", err, body)
	}
	if len(channels) == 0 {
		t.Error("expected the seeded channels")
	}
}

func startRealServer(t *testing.T) (*Server, string) {
	t.Helper()

	dir := t.TempDir()
	s, err := sqlite.Open(sqlite.MemoryPath)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	t.Cleanup(func() { s.Close() })

	srv, err := Start(Options{
		AppDir: dir,
		Port:   0,
		Config: service.NewConfig(s),
		Wallpaper: service.NewWallpaper(s, func([]string) (*channel.PictureMeta, error) {
			return nil, nil
		}, nil),
	})
	if err != nil {
		t.Fatalf("Start = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	})
	return srv, dir
}
