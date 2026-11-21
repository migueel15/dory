package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/api"
	"github.com/migueel15/dory/internal/auth"
	"github.com/migueel15/dory/internal/devices"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const deviceFlow = " Flow: device_ui to read the screen, act (device_tap, device_type, device_key...), then device_ui or device_screenshot to check the result. Everything the agent does is shown live to the user."

type deviceRef struct {
	Serial string `json:"serial,omitempty" jsonschema:"Device serial from device_list; optional when exactly one device is ready"`
}

type tapInput struct {
	deviceRef
	X     *int   `json:"x,omitempty" jsonschema:"Screen x in pixels (with y)"`
	Y     *int   `json:"y,omitempty"`
	Index *int   `json:"index,omitempty" jsonschema:"Element index from the last device_ui"`
	Text  string `json:"text,omitempty" jsonschema:"Text or content description of an element of the last device_ui"`
	ID    string `json:"id,omitempty" jsonschema:"Resource id of an element of the last device_ui"`
}

func addDeviceTools(srv *sdk.Server, s *api.Server) {
	m := s.Devices
	// pick resolves the device to act on: the given serial, or the only one ready.
	pick := func(ctx context.Context, serial string) (string, error) {
		if m.Tools.ADB == "" {
			return "", devices.ErrNoADB
		}
		list, err := m.ADB.Devices(ctx)
		if err != nil {
			return "", err
		}
		var ready []string
		for _, d := range list {
			if d.State == "device" {
				if serial != "" && d.Serial == serial {
					return serial, nil
				}
				ready = append(ready, d.Serial)
			}
		}
		if serial != "" {
			return "", fmt.Errorf("el dispositivo %q no está listo; usa device_list", serial)
		}
		if len(ready) == 1 {
			return ready[0], nil
		}
		return "", errors.New("indica el serial del dispositivo (device_list)")
	}
	note := func(ctx context.Context, action, serial, detail string) {
		// Tool calls already appear as cards in the thread; the log keeps a trace on the node too.
		thread, _ := ctx.Value(threadKey{}).(string)
		fmt.Printf("devices: thread=%s action=%s device=%s %s\n", thread, action, serial, detail)
	}
	act := func(name, desc string, fn func(ctx context.Context, serial string, in tapInput) (string, error)) {
		add(srv, name, desc+deviceFlow, func(ctx context.Context, in tapInput) (any, error) {
			serial, err := pick(ctx, in.Serial)
			if err != nil {
				return nil, err
			}
			res, err := fn(ctx, serial, in)
			if err != nil {
				return nil, err
			}
			note(ctx, name, serial, res)
			return map[string]any{"device": serial, "result": res}, nil
		})
	}

	add(srv, "device_list", "List the Android devices (phones by USB or Wi-Fi, running emulators) and the emulators (AVD) this node can start."+deviceFlow, func(ctx context.Context, _ struct{}) (any, error) {
		if m.Tools.ADB == "" {
			return nil, devices.ErrNoADB
		}
		list, err := m.ADB.Devices(ctx)
		if err != nil {
			return nil, err
		}
		for i, d := range list {
			if d.Transport == "emulator" && d.State == "device" {
				list[i].AVD = m.ADB.AVDName(ctx, d.Serial)
			}
		}
		return map[string]any{"devices": list, "avds": devices.ListAVDs(ctx, m.Tools), "avd_states": m.AVDStates()}, nil
	})
	add(srv, "device_open", "Show a device in this thread's side panel so the user watches what the agent does. Give a serial, or an avd name to start that emulator (it waits until Android booted, up to two minutes). With floating the device opens over the chat."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		AVD      string `json:"avd,omitempty" jsonschema:"Name of an emulator to start and open"`
		Floating bool   `json:"floating,omitempty"`
	}) (any, error) {
		thread, _ := ctx.Value(threadKey{}).(string)
		serial := in.Serial
		if in.AVD != "" {
			if err := m.StartAVD(in.AVD); err != nil {
				return nil, err
			}
			deadline := time.Now().Add(2 * time.Minute)
			for serial == "" {
				if st := m.AVDStates()[in.AVD]; st.Status == "failed" {
					return nil, errors.New(st.Error)
				}
				if list, err := m.ADB.Devices(ctx); err == nil {
					for _, d := range list {
						if d.Transport == "emulator" && d.State == "device" && m.ADB.AVDName(ctx, d.Serial) == in.AVD && m.AVDStates()[in.AVD].Status == "ready" {
							serial = d.Serial
						}
					}
				}
				if serial == "" {
					if time.Now().After(deadline) {
						return nil, errors.New("el emulador no terminó de arrancar; vuelve a intentarlo")
					}
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(2 * time.Second):
					}
				}
			}
		}
		serial, err := pick(ctx, serial)
		if err != nil {
			return nil, err
		}
		if thread != "" && s.Events != nil {
			s.Events.Notify(api.Change{Entity: "device", ID: thread, Data: map[string]any{"action": "open", "serial": serial, "floating": in.Floating}})
		}
		note(ctx, "device_open", serial, "")
		return map[string]any{"device": serial, "opened_in_thread": thread != ""}, nil
	})
	add(srv, "device_close", "Close the device tab of this thread, or turn off an emulator with stop."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Stop bool `json:"stop,omitempty" jsonschema:"Also shut the emulator down (physical phones are never powered off by agents)"`
	}) (any, error) {
		thread, _ := ctx.Value(threadKey{}).(string)
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if in.Stop {
			if !strings.HasPrefix(serial, "emulator-") {
				return nil, errors.New("los agentes solo pueden apagar emuladores")
			}
			if err := m.Stop(ctx, serial); err != nil {
				return nil, err
			}
		}
		if thread != "" && s.Events != nil {
			s.Events.Notify(api.Change{Entity: "device", ID: thread, Data: map[string]any{"action": "close", "serial": serial}})
		}
		note(ctx, "device_close", serial, "")
		return map[string]any{"device": serial, "closed": true}, nil
	})

	// The screenshot comes back as an image, so it does not go through add.
	sdk.AddTool(srv, &sdk.Tool{Name: "device_screenshot", Description: "Take a screenshot of a device, scaled down to at most 900 px. Secure screens (banking, passwords) come out black; then rely on coordinates." + deviceFlow},
		func(ctx context.Context, _ *sdk.CallToolRequest, in deviceRef) (*sdk.CallToolResult, any, error) {
			if !auth.Allowed(ctx, "write") {
				return nil, nil, errors.New(`permiso "write" requerido`)
			}
			serial, err := pick(ctx, in.Serial)
			if err != nil {
				return nil, nil, err
			}
			png, w, h, err := m.Screenshot(ctx, serial, 900)
			if err != nil {
				return nil, nil, err
			}
			note(ctx, "device_screenshot", serial, "")
			return &sdk.CallToolResult{Content: []sdk.Content{
				&sdk.ImageContent{Data: png, MIMEType: "image/png"},
				&sdk.TextContent{Text: fmt.Sprintf("device %s, real screen %dx%d px; coordinates for device_tap use the real size", serial, w, h)},
			}}, nil, nil
		})

	add(srv, "device_ui", "Read the screen as a compact list of elements (index, text, id, desc, bounds and center x,y). Takes 1-3 s and fails on secure screens; then use device_screenshot."+deviceFlow, func(ctx context.Context, in deviceRef) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		els, err := m.UI(ctx, serial)
		if err != nil {
			return nil, err
		}
		note(ctx, "device_ui", serial, fmt.Sprintf("%d elements", len(els)))
		return map[string]any{"device": serial, "elements": els}, nil
	})
	act("device_tap", "Tap the center of an element from the last device_ui (index, text or id) or a point (x, y).", func(ctx context.Context, serial string, in tapInput) (string, error) {
		x, y, what, err := m.Resolve(serial, devices.Target{X: in.X, Y: in.Y, Index: in.Index, Text: in.Text, ID: in.ID})
		if err != nil {
			return "", err
		}
		if err := m.Tap(ctx, serial, x, y); err != nil {
			return "", err
		}
		return fmt.Sprintf("tapped %d,%d %s", x, y, what), nil
	})
	add(srv, "device_swipe", "Swipe between two points, e.g. to scroll (swipe up to move down the list)."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		X1         int `json:"x1"`
		Y1         int `json:"y1"`
		X2         int `json:"x2"`
		Y2         int `json:"y2"`
		DurationMS int `json:"duration_ms,omitempty"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if err := m.Swipe(ctx, serial, in.X1, in.Y1, in.X2, in.Y2, max(in.DurationMS, 300)); err != nil {
			return nil, err
		}
		note(ctx, "device_swipe", serial, "")
		return map[string]any{"device": serial, "result": "swiped"}, nil
	})
	add(srv, "device_type", "Type text into the focused field. Tap the field first. Non-ASCII text needs the device open in the panel (device_open)."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Text string `json:"text"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if err := m.InputText(ctx, serial, in.Text); err != nil {
			return nil, err
		}
		note(ctx, "device_type", serial, fmt.Sprintf("%d chars", len([]rune(in.Text))))
		return map[string]any{"device": serial, "result": "typed"}, nil
	})
	add(srv, "device_key", "Press a key: "+strings.Join(sortedKeys(), ", ")+"."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Key string `json:"key"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if err := m.Key(ctx, serial, in.Key); err != nil {
			return nil, err
		}
		note(ctx, "device_key", serial, in.Key)
		return map[string]any{"device": serial, "result": "pressed " + in.Key}, nil
	})
	add(srv, "device_launch", "Open an app by its name (\"WhatsApp\", \"Ajustes\") or exact package. Names are matched against the apps with a launcher icon; several matches return the candidates."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Name    string `json:"name,omitempty" jsonschema:"App name as people call it"`
		Package string `json:"package,omitempty" jsonschema:"Exact package name, instead of name"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		query := firstOf(in.Name, in.Package)
		if query == "" {
			return nil, errors.New("indica name o package")
		}
		app, err := m.LaunchByName(ctx, serial, query)
		if err != nil {
			return nil, err
		}
		note(ctx, "device_launch", serial, app.Package)
		return map[string]any{"device": serial, "result": "launched " + app.Name, "package": app.Package}, nil
	})
	add(srv, "device_open_url", "Open a link or deep link (https://..., whatsapp://send?phone=..., geo:...) with the app that handles it."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		URL string `json:"url"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if err := m.OpenURL(ctx, serial, in.URL); err != nil {
			return nil, err
		}
		note(ctx, "device_open_url", serial, in.URL)
		return map[string]any{"device": serial, "result": "opened " + in.URL}, nil
	})
	add(srv, "device_apps", "List installed apps. By default the apps with a launcher icon, each with package, main activity and a readable name; with all, every package (user apps unless system is true). filter narrows by name or package."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		All    bool   `json:"all,omitempty" jsonschema:"List raw packages instead of launcher apps"`
		System bool   `json:"system,omitempty" jsonschema:"With all, include system packages"`
		Filter string `json:"filter,omitempty"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if in.All {
			pkgs, err := m.Apps(ctx, serial, in.System, in.Filter)
			if err != nil {
				return nil, err
			}
			return map[string]any{"device": serial, "packages": pkgs}, nil
		}
		apps, err := m.LauncherApps(ctx, serial)
		if err != nil {
			return nil, err
		}
		if in.Filter != "" {
			apps = devices.FindApp(apps, in.Filter)
		}
		return map[string]any{"device": serial, "apps": apps}, nil
	})
	add(srv, "device_scroll", "Scroll the content: down shows what is below, up what is above, left/right for carousels. No coordinates needed."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Direction string `json:"direction" jsonschema:"up, down, left or right"`
		Times     int    `json:"times,omitempty" jsonschema:"Repeat, default 1, at most 10"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		for i := 0; i < min(max(in.Times, 1), 10); i++ {
			if err := m.Scroll(ctx, serial, in.Direction); err != nil {
				return nil, err
			}
			time.Sleep(250 * time.Millisecond)
		}
		note(ctx, "device_scroll", serial, in.Direction)
		return map[string]any{"device": serial, "result": "scrolled " + in.Direction}, nil
	})
	act("device_long_press", "Press and hold an element of the last device_ui (index, text or id) or a point (x, y), e.g. to select a message or open a context menu.", func(ctx context.Context, serial string, in tapInput) (string, error) {
		x, y, what, err := m.Resolve(serial, devices.Target{X: in.X, Y: in.Y, Index: in.Index, Text: in.Text, ID: in.ID})
		if err != nil {
			return "", err
		}
		if err := m.LongPress(ctx, serial, x, y); err != nil {
			return "", err
		}
		return fmt.Sprintf("long pressed %d,%d %s", x, y, what), nil
	})
	add(srv, "device_tap_text", "Read the screen and tap the first element whose text or description matches, in one call. Prefer it over device_ui + device_tap for a known label."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Text string `json:"text"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if _, err := m.UI(ctx, serial); err != nil {
			return nil, err
		}
		x, y, what, err := m.Resolve(serial, devices.Target{Text: in.Text})
		if err != nil {
			return nil, err
		}
		if err := m.Tap(ctx, serial, x, y); err != nil {
			return nil, err
		}
		note(ctx, "device_tap_text", serial, in.Text)
		return map[string]any{"device": serial, "result": fmt.Sprintf("tapped %q at %d,%d", what, x, y)}, nil
	})
	add(srv, "device_find", "Check whether a text is on screen and where, without an image."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Text string `json:"text"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		els, err := m.Find(ctx, serial, in.Text)
		if err != nil {
			return nil, err
		}
		return map[string]any{"device": serial, "found": len(els) > 0, "elements": els}, nil
	})
	add(srv, "device_wait_for", "Wait until a text appears on screen (polls every second, default 10 s, at most 30 s)."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		Text    string `json:"text"`
		Seconds int    `json:"seconds,omitempty"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		secs := min(max(in.Seconds, 1), 30)
		if in.Seconds == 0 {
			secs = 10
		}
		els, ok, err := m.WaitFor(ctx, serial, in.Text, time.Duration(secs)*time.Second)
		if err != nil {
			return nil, err
		}
		note(ctx, "device_wait_for", serial, in.Text)
		return map[string]any{"device": serial, "found": ok, "elements": els}, nil
	})
	add(srv, "device_state", "Read the device state as text: app in front, screen on/off, battery, charging, dark mode, text size, orientation, Wi-Fi and data."+deviceFlow, func(ctx context.Context, in deviceRef) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		return map[string]any{"device": serial, "state": m.Overview(ctx, serial)}, nil
	})
	add(srv, "device_notifications", "List the active notifications (app, title, text)."+deviceFlow, func(ctx context.Context, in deviceRef) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		list, err := m.Notifications(ctx, serial)
		if err != nil {
			return nil, err
		}
		return map[string]any{"device": serial, "notifications": list}, nil
	})
	add(srv, "device_screen", "Turn the screen on or off, or unlock a lock screen that has no PIN (state: on, off, unlock)."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		State string `json:"state"`
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if err := m.Screen(ctx, serial, in.State); err != nil {
			return nil, err
		}
		note(ctx, "device_screen", serial, in.State)
		return map[string]any{"device": serial, "result": "screen " + in.State}, nil
	})
	add(srv, "device_setting", "Change a device setting and read the value back. action: dark_mode (yes|no|auto), font_scale (0.5-2.0), orientation (auto|portrait|landscape|reverse_portrait|reverse_landscape), wifi and data (on|off), permission (package, permission, value grant|revoke), location (lat, lon; emulators only)."+deviceFlow, func(ctx context.Context, in struct {
		deviceRef
		devices.Action
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if in.Action.Name == "state" || in.Action.Name == "" {
			return nil, errors.New("indica una acción; para leer el estado usa device_state")
		}
		out, err := m.Apply(ctx, serial, in.Action)
		if err != nil {
			return nil, err
		}
		note(ctx, "device_setting", serial, in.Action.Name)
		return map[string]any{"device": serial, "value": out}, nil
	})
}

func firstOf(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func sortedKeys() []string {
	k := devices.KeyNames()
	for i := range k {
		for j := i + 1; j < len(k); j++ {
			if k[j] < k[i] {
				k[i], k[j] = k[j], k[i]
			}
		}
	}
	return k
}
