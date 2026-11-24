package devices

import (
	"context"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/i18n"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// App is an installed app with a launcher icon.
type App struct {
	Package  string `json:"package"`
	Activity string `json:"activity"`
	Name     string `json:"name,omitempty"` // best guess of the visible name
}

// knownApps maps the names people use to the packages that answer to them.
var knownApps = map[string][]string{
	"whatsapp": {"com.whatsapp"}, "whatsapp business": {"com.whatsapp.w4b"}, "chrome": {"com.android.chrome"}, "gmail": {"com.google.android.gm"},
	"youtube": {"com.google.android.youtube"}, "maps": {"com.google.android.apps.maps"}, "mapas": {"com.google.android.apps.maps"},
	"ajustes": {"com.android.settings"}, "settings": {"com.android.settings"}, "configuración": {"com.android.settings"},
	"cámara": {"com.android.camera", "com.google.android.GoogleCamera", "com.android.camera2"}, "camera": {"com.android.camera", "com.google.android.GoogleCamera", "com.android.camera2"},
	"teléfono": {"com.google.android.dialer", "com.android.dialer", "com.android.contacts"}, "phone": {"com.google.android.dialer", "com.android.dialer"},
	"mensajes": {"com.google.android.apps.messaging", "com.android.mms"}, "messages": {"com.google.android.apps.messaging", "com.android.mms"},
	"telegram": {"org.telegram.messenger"}, "instagram": {"com.instagram.android"}, "spotify": {"com.spotify.music"},
	"play store": {"com.android.vending"}, "tienda": {"com.android.vending"}, "fotos": {"com.google.android.apps.photos"}, "photos": {"com.google.android.apps.photos"},
	"calendario": {"com.google.android.calendar"}, "calendar": {"com.google.android.calendar"}, "contactos": {"com.google.android.contacts", "com.android.contacts"},
	"calculadora": {"com.google.android.calculator", "com.miui.calculator", "com.android.calculator2"}, "reloj": {"com.google.android.deskclock", "com.android.deskclock"},
	"twitter": {"com.twitter.android"}, "x": {"com.twitter.android"}, "facebook": {"com.facebook.katana"}, "tiktok": {"com.zhiliaoapp.musically"},
	"netflix": {"com.netflix.mediaclient"}, "drive": {"com.google.android.apps.docs"}, "archivos": {"com.google.android.apps.nbu.files", "com.android.documentsui"},
	"notas": {"com.google.android.keep"}, "keep": {"com.google.android.keep"}, "linkedin": {"com.linkedin.android"}, "discord": {"com.discord"},
}

var queryRe = regexp.MustCompile(`^\s+([A-Za-z0-9_.]+)/([A-Za-z0-9_.$]+)\s*$`)

// ParseLauncher reads `cmd package query-activities --brief` output.
func ParseLauncher(out string) []App {
	var apps []App
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if m := queryRe.FindStringSubmatch(l); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			apps = append(apps, App{Package: m[1], Activity: m[2], Name: guessName(m[1])})
		}
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Package < apps[j].Package })
	return apps
}

// guessName gives a readable name: the known one, or the last meaningful part of the package.
func guessName(pkg string) string {
	for name, pkgs := range knownApps {
		for _, p := range pkgs {
			if p == pkg && len(name) > 2 {
				return strings.ToUpper(name[:1]) + name[1:]
			}
		}
	}
	parts := strings.Split(pkg, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		if p := parts[i]; p != "android" && p != "app" && p != "apps" && p != "mobile" && len(p) > 1 {
			return strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return pkg
}

// LauncherApps lists the apps that show an icon in the launcher.
func (m *Manager) LauncherApps(ctx context.Context, serial string) ([]App, error) {
	out, err := m.ADB.Shell(ctx, serial, 12*time.Second, "cmd", "package", "query-activities", "--brief", "-a", "android.intent.action.MAIN", "-c", "android.intent.category.LAUNCHER")
	if err != nil {
		return nil, err
	}
	return ParseLauncher(out), nil
}

// FindApp resolves what the user called an app to the launchable apps it can be.
func FindApp(apps []App, query string) []App {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	var exact, loose []App
	known := map[string]bool{}
	for _, p := range knownApps[q] {
		known[p] = true
	}
	if len(known) == 0 { // "play" finds "play store" only when nothing is called "play"
		for name, pkgs := range knownApps {
			if len(q) > 3 && strings.Contains(name, q) {
				for _, p := range pkgs {
					known[p] = true
				}
			}
		}
	}
	for _, a := range apps {
		pkg, name := strings.ToLower(a.Package), strings.ToLower(a.Name)
		switch {
		case pkg == q || name == q || known[a.Package]:
			exact = append(exact, a)
		case strings.Contains(name, q) || strings.Contains(pkg, q):
			loose = append(loose, a)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return loose
}

// LaunchByName opens an app by its visible name or package; several matches are an error that lists them.
func (m *Manager) LaunchByName(ctx context.Context, serial, query string) (App, error) {
	apps, err := m.LauncherApps(ctx, serial)
	if err != nil {
		return App{}, err
	}
	found := FindApp(apps, query)
	switch len(found) {
	case 0:
		return App{}, fmt.Errorf(i18n.Text("devices.no_hay_ninguna_app"), query)
	case 1:
		a := found[0]
		out, err := m.ADB.Shell(ctx, serial, 10*time.Second, "am", "start", "-n", a.Package+"/"+a.Activity)
		if err == nil && strings.Contains(out, "Error") {
			err = errors.New(out)
		}
		return a, err
	}
	var names []string
	for _, a := range found {
		names = append(names, a.Name+" ("+a.Package+")")
	}
	return App{}, fmt.Errorf(i18n.Text("devices.coincide_varias_apps_repite"), query, strings.Join(names, ", "))
}

// ScreenSize reads the device's screen size in pixels.
func (m *Manager) ScreenSize(ctx context.Context, serial string) (int, int, error) {
	out, err := m.ADB.Shell(ctx, serial, 5*time.Second, "wm", "size")
	if err != nil {
		return 0, 0, err
	}
	w, h := 0, 0
	for _, l := range strings.Split(out, "\n") {
		if _, rest, ok := strings.Cut(l, "size:"); ok { // an override line comes last and wins
			ws, hs, _ := strings.Cut(strings.TrimSpace(rest), "x")
			w, _ = strconv.Atoi(ws)
			h, _ = strconv.Atoi(hs)
		}
	}
	if w == 0 || h == 0 {
		return 0, 0, errors.New(i18n.Text("devices.no_pudo_leer_tamano"))
	}
	return w, h, nil
}

// Scroll moves the content in a direction (down shows what is below) from the middle of the screen.
func (m *Manager) Scroll(ctx context.Context, serial, dir string) error {
	w, h, err := m.ScreenSize(ctx, serial)
	if err != nil {
		return err
	}
	cx, cy := w/2, h/2
	dx, dy := w/3, h/3
	switch dir {
	case "down":
		return m.Swipe(ctx, serial, cx, cy+dy, cx, cy-dy, 350)
	case "up":
		return m.Swipe(ctx, serial, cx, cy-dy, cx, cy+dy, 350)
	case "right":
		return m.Swipe(ctx, serial, cx+dx, cy, cx-dx, cy, 350)
	case "left":
		return m.Swipe(ctx, serial, cx-dx, cy, cx+dx, cy, 350)
	}
	return errors.New(i18n.Text("devices.direccion_debe_ser_up"))
}

// LongPress holds a point for about a second.
func (m *Manager) LongPress(ctx context.Context, serial string, x, y int) error {
	return m.Swipe(ctx, serial, x, y, x, y, 1000)
}

// Find reads the screen and returns the elements whose text or description contains text.
func (m *Manager) Find(ctx context.Context, serial, text string) ([]UIElement, error) {
	els, err := m.UI(ctx, serial)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(text)
	var out []UIElement
	for _, e := range els {
		if strings.Contains(strings.ToLower(e.Text), q) || strings.Contains(strings.ToLower(e.Desc), q) {
			out = append(out, e)
		}
	}
	return out, nil
}

// WaitFor polls the screen until text shows up or the timeout ends.
func (m *Manager) WaitFor(ctx context.Context, serial, text string, timeout time.Duration) ([]UIElement, bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		els, err := m.Find(ctx, serial, text)
		if err == nil && len(els) > 0 {
			return els, true, nil
		}
		if time.Now().After(deadline) {
			return nil, false, nil
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// Notification is an active notification.
type Notification struct {
	Package string `json:"package"`
	Title   string `json:"title,omitempty"`
	Text    string `json:"text,omitempty"`
}

var (
	notifRe = regexp.MustCompile(`NotificationRecord\([0-9a-fx]+: pkg=([A-Za-z0-9_.]+)`)
	extraRe = regexp.MustCompile(`android\.(title|text)=\w+ \((.*)\)\s*$`)
)

// ParseNotifications reads `dumpsys notification --noredact`.
func ParseNotifications(dump string) []Notification {
	var out []Notification
	for _, l := range strings.Split(dump, "\n") {
		if m := notifRe.FindStringSubmatch(l); m != nil {
			out = append(out, Notification{Package: m[1]})
			continue
		}
		if len(out) == 0 {
			continue
		}
		if m := extraRe.FindStringSubmatch(strings.TrimRight(l, "\r")); m != nil {
			n := &out[len(out)-1]
			if m[1] == "title" {
				n.Title = m[2]
			} else {
				n.Text = m[2]
			}
		}
	}
	var withText []Notification
	for _, n := range out {
		if n.Title != "" || n.Text != "" {
			withText = append(withText, n)
		}
	}
	return withText
}

// Notifications lists the active notifications that have a title or text.
func (m *Manager) Notifications(ctx context.Context, serial string) ([]Notification, error) {
	out, err := m.ADB.Shell(ctx, serial, 10*time.Second, "dumpsys", "notification", "--noredact")
	if err != nil {
		return nil, err
	}
	return ParseNotifications(out), nil
}

// Screen turns the display on or off, or wakes it and swipes past a lock screen without a PIN.
func (m *Manager) Screen(ctx context.Context, serial, state string) error {
	switch state {
	case "on":
		return m.Key(ctx, serial, "wakeup")
	case "off":
		return m.Key(ctx, serial, "sleep")
	case "unlock":
		if err := m.Key(ctx, serial, "wakeup"); err != nil {
			return err
		}
		time.Sleep(600 * time.Millisecond)
		w, h, err := m.ScreenSize(ctx, serial)
		if err != nil {
			return err
		}
		return m.Swipe(ctx, serial, w/2, h*4/5, w/2, h/5, 300)
	}
	return errors.New(i18n.Text("devices.estado_debe_ser_on"))
}

// Overview reads the device state as text: what is in front, battery, screen and the settings of the tools drawer.
func (m *Manager) Overview(ctx context.Context, serial string) map[string]string {
	st := m.State(ctx, serial)
	if out, err := m.ADB.Shell(ctx, serial, 6*time.Second, "dumpsys", "battery"); err == nil {
		for _, l := range strings.Split(out, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(l), ": ")
			if ok && (k == "level" || k == "powered" || k == "AC powered" || k == "USB powered") {
				if v == "true" && k != "level" {
					st["charging"] = "yes"
				}
				if k == "level" {
					st["battery"] = v + "%"
				}
			}
		}
	}
	if out, err := m.ADB.Shell(ctx, serial, 6*time.Second, "dumpsys", "power"); err == nil {
		st["screen"] = "on"
		if strings.Contains(out, "mWakefulness=Asleep") || strings.Contains(out, "mWakefulness=Dozing") {
			st["screen"] = "off"
		}
	}
	return st
}
