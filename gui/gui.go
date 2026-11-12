// Package gui runs goTApaper's desktop interface: a tray icon and a window
// showing the web interface.
//
// It replaces the getlantern/systray tray with Wails v3, which brings a
// webview along with the tray. There is one process: the Wails application
// *is* the daemon, because the configuration database has a single writer and
// a separate GUI process would be a second one.
//
// The window does not load a bundled asset server. It navigates to the API
// server's own /ui/ and is just another API client, using the same bearer
// token any client does. Being this process's own window, it is handed that
// token natively on every page load rather than asking for it; a browser
// opened from the tray gets it through a single-use ticket instead.
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

	// InstanceID identifies the one GUI allowed to run. Launching a second
	// with the same ID shows the first one's window instead. It should vary
	// with the application directory: the limit exists because a database has
	// one writer, so two GUIs on two directories are fine.
	InstanceID string

	// SessionURL returns a URL that opens the interface in a browser already
	// signed in. It is called afresh every time, because the ticket it embeds
	// is single-use.
	SessionURL func() (string, error)
	// UIURL is the interface's plain address, which the window loads. The
	// window needs no ticket: it is handed the token directly (see Token).
	UIURL func() (string, error)
	// Token returns the API token, which the window's page is given each time
	// it loads, so the native window never asks its user to sign in.
	Token func() string

	// Refresh asks for a wallpaper refresh now. An empty slice means the
	// configured rotation.
	Refresh func(channels []string)

	// OnReady runs once the application is up, for starting the refresh loop.
	OnReady func(tray *Tray)
	// OnExit runs as the application shuts down.
	OnExit func()
	// AfterExit runs once shutdown has finished, the single-instance lock
	// included, for restarting. On macOS the process exits straight after it
	// returns, so it is the last place anything can happen.
	AfterExit func()

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
	// Assigned once the tray exists. A second launch can only be reported
	// after the event loop starts, by which time it has been.
	var tray *Tray

	appOptions := application.Options{
		Name:        opts.Name,
		Description: opts.Description,
		Mac: application.MacOptions{
			// Started as an accessory: a tray tool should not take over the
			// Dock or the menu bar until it has a window to show. Opening the
			// window switches to a regular application, so it can be found
			// with Cmd+Tab, and hiding it switches back. See activation.
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		WarningHandler: func(text string) {
			logrus.Warn(text)
		},
		ErrorHandler: func(err error) {
			logrus.WithError(err).Error("desktop interface error")
		},
		PostShutdown: opts.AfterExit,
	}
	if opts.InstanceID != "" {
		appOptions.SingleInstance = &application.SingleInstanceOptions{
			UniqueID: opts.InstanceID,
			// Launching it again is how a user asks to see it, so show it.
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if tray != nil {
					tray.OpenWindow()
				}
			},
		}
	}
	app := application.New(appOptions)

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
	// The page announces itself on the webview's native message channel once
	// it has loaded, which Wails reports as WindowRuntimeReady; it is answered
	// with the token. See signInScript for why this is safe, and auth.ts for
	// the page's side.
	window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		handOverToken(window, opts)
	})

	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		setForeground(false)
		e.Cancel()
	})

	tray = newTray(app, window, opts)

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
	} else if runtime.GOOS == "linux" && opts.Icon != nil {
		// Wails's Linux StatusNotifierItem implementation decodes the icon
		// with image/png.Decode, so extract the PNG frame from the .ico.
		t.tray.SetIcon(icoToPNG(opts.Icon))
	} else if opts.Icon != nil {
		t.tray.SetIcon(opts.Icon)
	}

	t.menu = app.Menu.New()
	t.buildMenu()
	t.tray.SetMenu(t.menu)

	// Clicking the icon toggles the interface window, which is what a user
	// expects of a tray application with a window.
	t.tray.OnClick(t.ToggleWindow)

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

	t.menu.Add("Toggle goTApaper Window").
		SetTooltip("Show or hide the configuration and history window").
		OnClick(func(*application.Context) { t.ToggleWindow() })

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

	t.menu.Add("Quit").OnClick(func(*application.Context) { t.Quit() })
}

// ToggleWindow hides the interface window when it is visible, and opens it
// when it is hidden.
func (t *Tray) ToggleWindow() {
	if t.window.IsVisible() {
		t.HideWindow()
		return
	}
	t.OpenWindow()
}

// HideWindow hides the interface window and returns macOS to accessory mode.
func (t *Tray) HideWindow() {
	t.window.Hide()
	setForeground(false)
}

// OpenWindow shows the interface. It reloads the page every time, which also
// re-signs it in: a page whose token was rejected, or that was signed out,
// comes back working.
func (t *Tray) OpenWindow() {
	if t.opts.UIURL == nil {
		logrus.WithError(errNoAPIServer).Error("cannot open the interface")
		return
	}
	url, err := t.opts.UIURL()
	if err != nil {
		logrus.WithError(err).Error("cannot open the interface")
		return
	}

	setForeground(true)
	t.window.SetURL(url)
	t.window.Show()
	t.window.Focus()
}

// handOverToken gives the window's page the API token.
func handOverToken(window application.Window, opts Options) {
	if opts.UIURL == nil || opts.Token == nil {
		return
	}
	uiURL, err := opts.UIURL()
	if err != nil {
		return
	}
	script, err := signInScript(uiURL, opts.Token())
	if err != nil {
		logrus.WithError(err).Error("cannot sign the window in")
		return
	}
	window.ExecJS(script)
}

// Quit shuts the application down, running OnExit and then AfterExit.
func (t *Tray) Quit() {
	t.app.Quit()
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
	// Consult t.installed() rather than t.startupItem.Checked(): Wails flips
	// the MenuItem's checked flag before running OnClick, so Checked() already
	// reports the post-click state rather than what is currently installed.
	if t.installed() {
		err = t.disableLogin()
	} else {
		err = t.enableLogin()
	}
	if err != nil {
		logrus.WithError(err).Error("cannot change the start-at-login setting")
	}
	// Read back rather than assuming the toggle took, so the checkbox cannot
	// disagree with the system. MenuItem setters update the native/D-Bus tray
	// menu directly on every platform; calling Menu.Update() on Linux would
	// replace the D-Bus tray item bindings with a GTK window menu.
	t.startupItem.SetChecked(t.installed())
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
}

// set gives a readout row its text, hiding it when there is nothing to say so
// the menu does not carry a run of blank lines.
func set(item *application.MenuItem, text string) {
	item.SetLabel(text)
	item.SetHidden(text == "")
}
