// Package gui runs goTApaper's desktop interface: a tray icon and a window
// showing the web interface.
//
// It replaces the getlantern/systray tray with Wails v3, which brings a
// webview along with the tray. There is one process: the Wails application
// *is* the daemon, because the configuration database has a single writer and
// a separate GUI process would be a second one.
//
// The window does not load a bundled asset server. It navigates to the API
// server's own /ui/, through the one-time ticket handshake that sets a session
// cookie. That choice is what keeps authentication to a single mechanism --
// the webview and a browser are the same origin presenting the same cookie --
// and means no API token is ever handed to JavaScript.
package gui

import (
	"runtime"

	"github.com/sirupsen/logrus"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/genzj/goTApaper/channel"
)

// Options configures the interface.
type Options struct {
	// Name and Description identify the application to the OS.
	Name        string
	Description string

	// SessionURL returns a URL that opens the interface with a session already
	// established. It is called afresh for every navigation, because the
	// ticket it embeds is single-use.
	SessionURL func() (string, error)

	// Refresh asks for a wallpaper refresh now. An empty slice means the
	// configured rotation.
	Refresh func(channels []string)

	// OnReady runs once the application is up, for starting the refresh loop.
	OnReady func(tray *Tray)
	// OnExit runs as the application shuts down.
	OnExit func()

	// Icons for the tray. TemplateIcon is a macOS monochrome template.
	Icon         []byte
	TemplateIcon []byte

	// StartAtLogin reports and sets whether the application launches at login.
	IsInstalled  func() bool
	EnableLogin  func() error
	DisableLogin func() error
}

// Run starts the interface and blocks until it exits.
//
// It must be called from the main goroutine: the webview and the tray are
// OS-level UI, and every platform requires their event loop to own the main
// thread.
func Run(opts Options) error {
	app := application.New(application.Options{
		Name:        opts.Name,
		Description: opts.Description,
		Mac: application.MacOptions{
			// An accessory application: a tray tool should not take over the
			// Dock or the menu bar, and closing its window should not quit it.
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  opts.Name,
		Width:  1100,
		Height: 800,
		// Started hidden: the application is a background wallpaper setter
		// that happens to have a window, so launching it must not steal focus.
		Hidden: true,
	})

	// Closing hides rather than quits, so the tray stays the only way out.
	// Without this, closing the window would end the refresh loop too.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	tray := newTray(app, window, opts)

	app.OnShutdown(func() {
		if opts.OnExit != nil {
			opts.OnExit()
		}
	})

	if opts.OnReady != nil {
		// Run after the event loop is up, so a callback that touches the tray
		// or the window is not racing their construction.
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
			opts.OnReady(tray)
		})
	}

	logrus.Debug("starting the desktop interface")
	return app.Run()
}

// Tray is the tray icon and its menu, with the parts that change at runtime.
type Tray struct {
	app    *application.App
	window application.Window
	opts   Options

	tray *application.SystemTray
	menu *application.Menu

	// Disabled rows showing the current picture, hidden until there is one.
	errorItem   *application.MenuItem
	titleItem   *application.MenuItem
	creditItem  *application.MenuItem
	channelItem *application.MenuItem
	timeItem    *application.MenuItem

	refreshItem *application.MenuItem
	startupItem *application.MenuItem
}

func newTray(app *application.App, window application.Window, opts Options) *Tray {
	t := &Tray{app: app, window: window, opts: opts}

	t.tray = app.SystemTray.New()
	t.tray.SetTooltip(opts.Name)
	if runtime.GOOS == "darwin" && opts.TemplateIcon != nil {
		// A template icon is what lets macOS invert it for a dark menu bar.
		t.tray.SetTemplateIcon(opts.TemplateIcon)
	} else if opts.Icon != nil {
		t.tray.SetIcon(opts.Icon)
	}

	t.menu = app.Menu.New()
	t.buildMenu()
	t.tray.SetMenu(t.menu)

	// Clicking the icon opens the interface, which is what a user expects of
	// a tray application with a window.
	t.tray.OnClick(t.OpenWindow)

	return t
}

func (t *Tray) buildMenu() {
	// Information about the current picture, mirroring what the old tray
	// showed. Disabled because they are a readout, not actions.
	t.errorItem = t.menu.Add("")
	t.errorItem.SetEnabled(false)
	t.errorItem.SetHidden(true)

	t.titleItem = t.menu.Add("")
	t.titleItem.SetEnabled(false)
	t.titleItem.SetHidden(true)

	t.creditItem = t.menu.Add("")
	t.creditItem.SetEnabled(false)
	t.creditItem.SetHidden(true)

	t.channelItem = t.menu.Add("")
	t.channelItem.SetEnabled(false)
	t.channelItem.SetHidden(true)

	t.timeItem = t.menu.Add("")
	t.timeItem.SetEnabled(false)
	t.timeItem.SetHidden(true)

	t.menu.AddSeparator()

	t.menu.Add("Open goTApaper").
		SetTooltip("Show the configuration and history window").
		OnClick(func(*application.Context) { t.OpenWindow() })

	t.menu.Add("Open in Browser").
		SetTooltip("Open the same interface in your default browser").
		OnClick(func(*application.Context) { t.OpenInBrowser() })

	t.menu.AddSeparator()

	t.startupItem = t.menu.AddCheckbox("Start at login", t.installed())
	t.startupItem.OnClick(func(*application.Context) { t.toggleStartup() })

	t.menu.AddSeparator()

	t.refreshItem = t.menu.Add("Refresh")
	t.refreshItem.SetTooltip("Download and apply a wallpaper now")
	t.refreshItem.OnClick(func(*application.Context) {
		if t.opts.Refresh != nil {
			t.opts.Refresh(nil)
		}
	})

	t.menu.Add("Quit").OnClick(func(*application.Context) { t.app.Quit() })
}

// OpenWindow shows the interface, navigating through a fresh session ticket.
//
// The URL is rebuilt every time because a ticket is single-use: reusing the
// one from the last open would be refused, and the window would show an error
// page instead of the interface.
func (t *Tray) OpenWindow() {
	url, err := t.sessionURL()
	if err != nil {
		logrus.WithError(err).Error("cannot open the interface")
		return
	}

	t.window.SetURL(url)
	t.window.Show()
	t.window.Focus()
}

// OpenInBrowser opens the interface in the user's default browser.
func (t *Tray) OpenInBrowser() {
	url, err := t.sessionURL()
	if err != nil {
		logrus.WithError(err).Error("cannot open the interface in a browser")
		return
	}
	if err := t.app.Browser.OpenURL(url); err != nil {
		logrus.WithError(err).Error("cannot launch the default browser")
	}
}

func (t *Tray) sessionURL() (string, error) {
	if t.opts.SessionURL == nil {
		return "", errNoAPIServer
	}
	return t.opts.SessionURL()
}

func (t *Tray) installed() bool {
	if t.opts.IsInstalled == nil {
		return false
	}
	return t.opts.IsInstalled()
}

func (t *Tray) toggleStartup() {
	var err error
	if t.startupItem.Checked() {
		err = t.disableLogin()
	} else {
		err = t.enableLogin()
	}
	if err != nil {
		logrus.WithError(err).Error("cannot change the start-at-login setting")
	}
	// Read back rather than assuming the toggle took, so the checkbox cannot
	// disagree with the system.
	t.startupItem.SetChecked(t.installed())
	t.menu.Update()
}

func (t *Tray) enableLogin() error {
	if t.opts.EnableLogin == nil {
		return nil
	}
	return t.opts.EnableLogin()
}

func (t *Tray) disableLogin() error {
	if t.opts.DisableLogin == nil {
		return nil
	}
	return t.opts.DisableLogin()
}

// Refreshing marks a refresh as in progress or finished.
func (t *Tray) Refreshing(active bool) {
	if active {
		t.refreshItem.SetLabel("Refreshing...")
		t.refreshItem.SetEnabled(false)
	} else {
		t.refreshItem.SetLabel("Refresh")
		t.refreshItem.SetEnabled(true)
	}
	t.menu.Update()
}

// ShowPicture updates the readout rows for a newly applied picture, or clears
// them when there is none.
func (t *Tray) ShowPicture(meta *channel.PictureMeta, err error) {
	if meta != nil {
		set(t.titleItem, meta.Title)
		set(t.creditItem, meta.Credit)
		set(t.channelItem, meta.Channel)
		set(t.timeItem, meta.UploadTime.Local().Format("2006-01-02 15:04"))
	} else {
		set(t.titleItem, "")
		set(t.creditItem, "")
		set(t.channelItem, "")
		set(t.timeItem, "")
	}

	if err != nil {
		set(t.errorItem, err.Error())
	} else {
		set(t.errorItem, "")
	}

	t.menu.Update()
}

// set gives a readout row its text, hiding it when there is nothing to say so
// the menu does not carry a run of blank lines.
func set(item *application.MenuItem, text string) {
	item.SetLabel(text)
	item.SetHidden(text == "")
}
