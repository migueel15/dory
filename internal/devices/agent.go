package devices

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"image"
	"image/png"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// UIElement is one element of the screen an agent can act on.
type UIElement struct {
	Index     int    `json:"index"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Desc      string `json:"desc,omitempty"`
	Class     string `json:"class,omitempty"`
	Clickable bool   `json:"clickable,omitempty"`
	Checked   bool   `json:"checked,omitempty"`
	Bounds    [4]int `json:"bounds"`
	CenterX   int    `json:"x"`
	CenterY   int    `json:"y"`
}

type uiNode struct {
	Text      string   `xml:"text,attr"`
	ID        string   `xml:"resource-id,attr"`
	Desc      string   `xml:"content-desc,attr"`
	Class     string   `xml:"class,attr"`
	Clickable string   `xml:"clickable,attr"`
	Checked   string   `xml:"checked,attr"`
	Bounds    string   `xml:"bounds,attr"`
	Children  []uiNode `xml:"node"`
}

var boundsRe = regexp.MustCompile(`^\[(\d+),(\d+)\]\[(\d+),(\d+)\]$`)

// ParseUI reduces a uiautomator dump to the elements with text, an id, a
// description or that can be clicked, each with its center.
func ParseUI(dump string) ([]UIElement, error) {
	if i := strings.Index(dump, "<?xml"); i > 0 {
		dump = dump[i:]
	}
	if i := strings.LastIndex(dump, "</hierarchy>"); i >= 0 {
		dump = dump[:i+len("</hierarchy>")]
	}
	if !strings.Contains(dump, "<hierarchy") {
		msg := strings.TrimSpace(dump)
		if msg == "" || strings.HasPrefix(msg, "UI hierchary") {
			msg = "la pantalla no devolvió interfaz"
		}
		return nil, fmt.Errorf("no se pudo leer la interfaz (pantalla animada, bloqueada o protegida; usa device_screenshot): %s", msg)
	}
	var root struct {
		Nodes []uiNode `xml:"node"`
	}
	if err := xml.Unmarshal([]byte(dump), &root); err != nil {
		return nil, fmt.Errorf("no se pudo leer la interfaz: %w", err)
	}
	var out []UIElement
	var walk func(n uiNode)
	walk = func(n uiNode) {
		clickable := n.Clickable == "true"
		if n.Text != "" || n.Desc != "" || n.ID != "" || clickable {
			if m := boundsRe.FindStringSubmatch(n.Bounds); m != nil {
				var b [4]int
				for i := range b {
					b[i], _ = strconv.Atoi(m[i+1])
				}
				if b[2] > b[0] && b[3] > b[1] && (n.Text != "" || n.Desc != "" || clickable) {
					id := n.ID
					if i := strings.Index(id, ":id/"); i >= 0 {
						id = id[i+4:]
					}
					class := n.Class
					if i := strings.LastIndex(class, "."); i >= 0 {
						class = class[i+1:]
					}
					out = append(out, UIElement{Index: len(out), Text: n.Text, ID: id, Desc: n.Desc, Class: class, Clickable: clickable, Checked: n.Checked == "true", Bounds: b, CenterX: (b[0] + b[2]) / 2, CenterY: (b[1] + b[3]) / 2})
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, n := range root.Nodes {
		walk(n)
	}
	return out, nil
}

// UI dumps the screen. It takes one to three seconds and fails on secure screens.
func (m *Manager) UI(ctx context.Context, serial string) ([]UIElement, error) {
	out, err := m.ADB.Run(ctx, 20*time.Second, "-s", serial, "shell", "uiautomator dump /data/local/tmp/dory-ui.xml >/dev/null 2>&1; cat /data/local/tmp/dory-ui.xml; rm -f /data/local/tmp/dory-ui.xml")
	if err != nil {
		return nil, err
	}
	els, err := ParseUI(string(out))
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.lastUI == nil {
		m.lastUI = map[string][]UIElement{}
	}
	m.lastUI[serial] = els
	m.mu.Unlock()
	return els, nil
}

// Target says where to tap: coordinates, or an element of the last UI by index, text or id.
type Target struct {
	X, Y  *int
	Index *int
	Text  string
	ID    string
}

// Resolve finds the point a target stands for in the last UI of a device.
func (m *Manager) Resolve(serial string, t Target) (int, int, string, error) {
	if t.X != nil && t.Y != nil {
		return *t.X, *t.Y, "", nil
	}
	m.mu.Lock()
	els := m.lastUI[serial]
	m.mu.Unlock()
	if len(els) == 0 {
		return 0, 0, "", errors.New("llama antes a device_ui, o usa coordenadas")
	}
	for _, e := range els {
		switch {
		case t.Index != nil && e.Index == *t.Index:
		case t.ID != "" && e.ID == t.ID:
		case t.Text != "" && (strings.EqualFold(e.Text, t.Text) || strings.EqualFold(e.Desc, t.Text)):
		default:
			continue
		}
		return e.CenterX, e.CenterY, firstNonEmpty(e.Text, e.Desc, e.ID), nil
	}
	return 0, 0, "", errors.New("no hay ningún elemento así en la última interfaz leída; vuelve a llamar a device_ui")
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// Tap touches a point.
func (m *Manager) Tap(ctx context.Context, serial string, x, y int) error {
	if x < 0 || y < 0 || x > 10000 || y > 10000 {
		return errors.New("coordenadas fuera de rango")
	}
	_, err := m.ADB.Shell(ctx, serial, 8*time.Second, "input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	return err
}

// Swipe drags between two points.
func (m *Manager) Swipe(ctx context.Context, serial string, x1, y1, x2, y2, ms int) error {
	ms = max(50, min(ms, 5000))
	_, err := m.ADB.Shell(ctx, serial, 10*time.Second, "input", "swipe", strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(x2), strconv.Itoa(y2), strconv.Itoa(ms))
	return err
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII || r < 32 {
			return false
		}
	}
	return true
}

// InputText types into the focused field. ASCII goes through `input text`;
// anything else needs an open scrcpy session, whose control channel injects it.
func (m *Manager) InputText(ctx context.Context, serial, text string) error {
	if text == "" || len(text) > 2000 {
		return errors.New("el texto debe tener entre 1 y 2000 caracteres")
	}
	if !isASCII(text) {
		if !m.Active(serial) {
			return errors.New("el texto con caracteres no ASCII necesita el dispositivo abierto en el panel (device_open)")
		}
		b := make([]byte, 5, 5+len(text))
		b[0] = CtlText
		binary.BigEndian.PutUint32(b[1:], uint32(len(text)))
		return m.Control(serial, append(b, text...))
	}
	// The device shell parses the line: quote it and use %s for spaces.
	arg := strings.ReplaceAll(text, " ", "%s")
	arg = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	_, err := m.ADB.Shell(ctx, serial, 10*time.Second, "input", "text", arg)
	return err
}

var keyCodes = map[string]string{"back": "4", "home": "3", "recents": "187", "enter": "66", "delete": "67", "tab": "61", "escape": "111", "up": "19", "down": "20", "left": "21", "right": "22",
	"power": "26", "volume_up": "24", "volume_down": "25", "search": "84", "menu": "82", "space": "62", "wakeup": "224"}

// KeyNames lists the keys device_key accepts.
func KeyNames() []string {
	out := make([]string, 0, len(keyCodes))
	for k := range keyCodes {
		out = append(out, k)
	}
	return out
}

// Key presses a named key.
func (m *Manager) Key(ctx context.Context, serial, name string) error {
	code, ok := keyCodes[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("tecla desconocida %q", name)
	}
	_, err := m.ADB.Shell(ctx, serial, 8*time.Second, "input", "keyevent", code)
	return err
}

// Launch opens an app by package name.
func (m *Manager) Launch(ctx context.Context, serial, pkg string) error {
	if !ident.MatchString(pkg) {
		return errors.New("paquete no válido")
	}
	out, err := m.ADB.Shell(ctx, serial, 10*time.Second, "monkey", "-p", pkg, "-c", "android.intent.category.LAUNCHER", "1")
	if err == nil && strings.Contains(out, "No activities found") {
		err = fmt.Errorf("%s no está instalada o no tiene pantalla de inicio", pkg)
	}
	return err
}

var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*:`)

// OpenURL opens a link or deep link with the app that handles it.
func (m *Manager) OpenURL(ctx context.Context, serial, url string) error {
	if !schemeRe.MatchString(url) || len(url) > 2000 || strings.ContainsFunc(url, unicode.IsControl) {
		return errors.New("enlace no válido: debe empezar por un esquema como https: o whatsapp:")
	}
	out, err := m.ADB.Shell(ctx, serial, 10*time.Second, "am", "start", "-a", "android.intent.action.VIEW", "-d", "'"+strings.ReplaceAll(url, "'", `'\''`)+"'")
	if err == nil && strings.Contains(out, "Error") {
		err = errors.New(out)
	}
	return err
}

// Apps lists installed packages; user apps only unless all is set.
func (m *Manager) Apps(ctx context.Context, serial string, all bool, filter string) ([]string, error) {
	args := []string{"pm", "list", "packages"}
	if !all {
		args = append(args, "-3")
	}
	out, err := m.ADB.Shell(ctx, serial, 10*time.Second, args...)
	if err != nil {
		return nil, err
	}
	var pkgs []string
	for _, l := range strings.Split(out, "\n") {
		p := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "package:"))
		if p != "" && (filter == "" || strings.Contains(strings.ToLower(p), strings.ToLower(filter))) {
			pkgs = append(pkgs, p)
		}
	}
	return pkgs, nil
}

// Screenshot captures the screen as a PNG whose longest side is at most maxSide.
func (m *Manager) Screenshot(ctx context.Context, serial string, maxSide int) ([]byte, int, int, error) {
	raw, err := m.ADB.Run(ctx, 15*time.Second, "-s", serial, "exec-out", "screencap", "-p")
	if err != nil {
		return nil, 0, 0, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, 0, errors.New("la captura no es una imagen válida (¿pantalla bloqueada o protegida?)")
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if maxSide > 0 && max(w, h) > maxSide {
		img = downscale(img, maxSide)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, 0, 0, err
		}
		raw = buf.Bytes()
	}
	return raw, w, h, nil
}

// downscale reduces an image with a box filter so its longest side is max.
func downscale(src image.Image, side int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	k := float64(side) / float64(max(w, h))
	nw, nh := max(1, int(float64(w)*k)), max(1, int(float64(h)*k))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max(y*h/nh+1, (y+1)*h/nh)
		for x := 0; x < nw; x++ {
			x0, x1 := x*w/nw, max(x*w/nw+1, (x+1)*w/nw)
			var r, g, bl, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := src.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), uint8(a/n>>8)
		}
	}
	return dst
}
