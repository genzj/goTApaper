package gui

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// signInScript is run in the window after every page load, handing the page
// the API token directly.
//
// The native window is this process's own UI, so asking its user for a token
// the process already holds is pointless -- and once they had signed out,
// getting back in meant knowing to reopen the window from the tray. Pushing
// the token in from Go needs no Wails binding, which a page served from the
// API server's own origin could not reliably reach anyway, and keeps the token
// out of every URL.
//
// The script checks the page's origin before handing anything over. It runs
// in whatever page the window happens to be showing, and only the interface
// served by this process may have the token.
func signInScript(uiURL, token string) (string, error) {
	parsed, err := url.Parse(uiURL)
	if err != nil {
		return "", err
	}
	origin := parsed.Scheme + "://" + parsed.Host

	// JSON-encoded, so neither value can break out of its string literal.
	o, err := json.Marshal(origin)
	if err != nil {
		return "", err
	}
	t, err := json.Marshal(token)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`(function () {
  if (window.location.origin !== %s) return;
  window.__gotapaperNative = { token: %s };
  window.dispatchEvent(new CustomEvent('gotapaper:native-sign-in'));
})();`, o, t), nil
}
