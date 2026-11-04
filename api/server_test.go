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

	"github.com/genzj/goTApaper/api/wire"
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
	srv, _ := startRealServer(t, nil)

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

	_, dir := startRealServer(t, nil)

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
	srv, dir := startRealServer(t, nil)

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
func TestLiveRequestRequiresTokenWhenStrict(t *testing.T) {
	srv, dir := startRealServer(t, func() bool { return true })

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

func startRealServer(t *testing.T, strict func() bool) (*Server, string) {
	t.Helper()

	dir := t.TempDir()
	s, err := sqlite.Open(sqlite.MemoryPath)
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	t.Cleanup(func() { s.Close() })

	srv, err := Start(Options{
		AppDir:    dir,
		Port:      0,
		Config:    service.NewConfig(s),
		Wallpaper: service.NewWallpaper(s),
		Downloader: service.NewDownloader(s, func([]string, bool) (*channel.PictureMeta, error) {
			return nil, nil
		}, nil),
		RequireToken: strict,
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

func TestAnEphemeralPortIsReusedOnTheNextStart(t *testing.T) {
	dir := t.TempDir()

	first, err := listen(dir, 0)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := first.Addr().(*net.TCPAddr).Port
	first.Close()

	// The interface's saved token is scoped to the origin, port included, so
	// a restart has to come back on the same port for it to survive.
	second, err := listen(dir, 0)
	if err != nil {
		t.Fatalf("listen again: %v", err)
	}
	defer second.Close()
	if got := second.Addr().(*net.TCPAddr).Port; got != port {
		t.Fatalf("second start got port %d, want the remembered %d", got, port)
	}
	if host := second.Addr().(*net.TCPAddr).IP.String(); host != LoopbackHost {
		t.Fatalf("bound %s, want %s", host, LoopbackHost)
	}
}

func TestATakenRememberedPortFallsBackToAFreshOne(t *testing.T) {
	dir := t.TempDir()

	held, err := listen(dir, 0)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer held.Close()
	taken := held.Addr().(*net.TCPAddr).Port

	// Still bound, so the remembered port is unavailable.
	other, err := listen(dir, 0)
	if err != nil {
		t.Fatalf("listen with the remembered port taken: %v", err)
	}
	defer other.Close()
	if got := other.Addr().(*net.TCPAddr).Port; got == taken {
		t.Fatal("bound the port another listener holds")
	}
}

func TestAnUnusableRememberedPortFallsBackToAFreshOne(t *testing.T) {
	// A damaged or hand-edited port file must not stop the server starting.
	// (A remembered port that is merely taken is covered above; whether a
	// low port is bindable at all differs by OS, so it is not relied on.)
	for _, content := range []string{"not-a-port\n", "70000\n", "-5\n", ""} {
		dir := t.TempDir()
		if err := os.WriteFile(wire.PortPath(dir), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}

		l, err := listen(dir, 0)
		if err != nil {
			t.Fatalf("port file %q: listen: %v", content, err)
		}
		port := l.Addr().(*net.TCPAddr).Port
		l.Close()
		if port <= 0 {
			t.Fatalf("port file %q: bound port %d", content, port)
		}

		// The fresh port is remembered in place of the unusable value.
		if got := rememberedPort(dir); got != port {
			t.Errorf("port file %q: remembered %d, want %d", content, got, port)
		}
	}
}

func TestLiveLocalRequestNeedsNoTokenByDefault(t *testing.T) {
	srv, _ := startRealServer(t, nil)
	base := "http://" + srv.Addr()

	// A real connection, so it really does come from 127.0.0.1.
	resp, err := http.Get(base + "/api/v1/channels")
	if err != nil {
		t.Fatalf("GET = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("tokenless local status = %d, want 200", resp.StatusCode)
	}

	// The same request, but as a reverse proxy on this machine would pass on
	// a remote user's: it arrives from 127.0.0.1 too, and must not get in.
	for _, header := range []string{"X-Forwarded-For", "Forwarded", "X-Real-IP", "X-Forwarded-Host"} {
		req, _ := http.NewRequest("GET", base+"/api/v1/channels", nil)
		req.Header.Set(header, "203.0.113.7")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET with %s = %v", header, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("forwarded (%s) status = %d, want 401", header, resp.StatusCode)
		}
	}

	// A wrong token is reported rather than ignored, even locally, so a
	// misconfigured client finds out.
	req, _ := http.NewRequest("GET", base+"/api/v1/channels", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong-token status = %d, want 401", resp.StatusCode)
	}
}

func TestLiveLocalCrossOriginWriteIsStillRefused(t *testing.T) {
	srv, _ := startRealServer(t, nil)

	// With no token needed locally, the Origin check is what keeps a web page
	// in the user's browser from rewriting the configuration.
	req, _ := http.NewRequest("PUT", "http://"+srv.Addr()+"/api/v1/settings/log.debug",
		strings.NewReader(`{"value":"true"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin write status = %d, want 403", resp.StatusCode)
	}
}
