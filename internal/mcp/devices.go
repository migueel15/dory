package mcp

import (
	"context"
	"errors"
	"fmt"
	"github.com/migueel15/dory/internal/apperr"
	"strings"
	"time"

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

func addDeviceTools(srv *sdk.Server, s *Deps) {
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
			return "", apperr.Newf(apperr.Unavailable, "device %q is not ready; use device_list", serial)
		}
		if len(ready) == 1 {
			return ready[0], nil
		}
		return "", apperr.New(apperr.Invalid, "pass the device serial (see device_list)")
	}
	note := func(ctx context.Context, action, serial, detail string) {
		// Tool calls already appear as cards in the thread; the log keeps a trace on the node too.
		thread, _ := ctx.Value(threadKey{}).(string)
		fmt.Printf("devices: thread=%s action=%s device=%s %s\n", thread, action, serial, detail)
	}
	act := func(name, desc string, fn func(ctx context.Context, serial string, in tapInput) (string, error)) {
		add(srv, name, desc, func(ctx context.Context, in tapInput) (any, error) {
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

	add(srv, "device_list", "List Android devices and emulators (AVD) this node can use."+deviceFlow, func(ctx context.Context, _ struct{}) (any, error) {
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
	add(srv, "device_open", "Show a device in this thread's panel; with avd, start that emulator first.", func(ctx context.Context, in struct {
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
						return nil, apperr.New(apperr.Unavailable, "the emulator did not finish booting; try again")
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
		if thread != "" && s.Notify != nil {
			s.Notify("device", thread, map[string]any{"action": "open", "serial": serial, "floating": in.Floating})
		}
		note(ctx, "device_open", serial, "")
		return map[string]any{"device": serial, "opened_in_thread": thread != ""}, nil
	})
	add(srv, "device_close", "Close this thread's device tab, or turn off an emulator with stop.", func(ctx context.Context, in struct {
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
				return nil, apperr.New(apperr.Forbidden, "agents can only power off emulators")
			}
			if err := m.Stop(ctx, serial); err != nil {
				return nil, err
			}
		}
		if thread != "" && s.Notify != nil {
			s.Notify("device", thread, map[string]any{"action": "close", "serial": serial})
		}
		note(ctx, "device_close", serial, "")
		return map[string]any{"device": serial, "closed": true}, nil
	})

	// The screenshot comes back as an image, so it does not go through add.
	sdk.AddTool(srv, &sdk.Tool{Name: "device_screenshot", Description: "Capture a device screen (up to 900 px). Use it when device_ui fails; secure screens come out black."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in deviceRef) (*sdk.CallToolResult, any, error) {
			if !auth.Allowed(ctx, "write") {
				return toolError(apperr.New(apperr.Forbidden, "the write scope is required")), nil, nil
			}
			serial, err := pick(ctx, in.Serial)
			if err != nil {
				return toolError(err), nil, nil
			}
			png, w, h, err := m.Screenshot(ctx, serial, 900)
			if err != nil {
				return toolError(err), nil, nil
			}
			note(ctx, "device_screenshot", serial, "")
			return &sdk.CallToolResult{Content: []sdk.Content{
				&sdk.ImageContent{Data: png, MIMEType: "image/png"},
				&sdk.TextContent{Text: fmt.Sprintf("device %s, real screen %dx%d px; coordinates for device_tap use the real size", serial, w, h)},
			}}, nil, nil
		})

	add(srv, "device_ui", "Read the screen as a list of elements with index, text and x, y. Fails on secure screens: use device_screenshot.", func(ctx context.Context, in deviceRef) (any, error) {
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
	act("device_tap", "Tap an element of the last device_ui (index, text or id) or a point x, y.", func(ctx context.Context, serial string, in tapInput) (string, error) {
		x, y, what, err := m.Resolve(serial, devices.Target{X: in.X, Y: in.Y, Index: in.Index, Text: in.Text, ID: in.ID})
		if err != nil {
			return "", err
		}
		if err := m.Tap(ctx, serial, x, y); err != nil {
			return "", err
		}
		return fmt.Sprintf("tapped %d,%d %s", x, y, what), nil
	})
	add(srv, "device_swipe", "Swipe between two points.", func(ctx context.Context, in struct {
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
	add(srv, "device_type", "Type into the focused field; tap it first.", func(ctx context.Context, in struct {
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
	add(srv, "device_key", "Press a key: "+strings.Join(sortedKeys(), ", ")+".", func(ctx context.Context, in struct {
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
	add(srv, "device_launch", "Open an app by name (\"WhatsApp\") or package.", func(ctx context.Context, in struct {
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
			return nil, apperr.New(apperr.Invalid, "pass name or package")
		}
		app, err := m.LaunchByName(ctx, serial, query)
		if err != nil {
			return nil, err
		}
		note(ctx, "device_launch", serial, app.Package)
		return map[string]any{"device": serial, "result": "launched " + app.Name, "package": app.Package}, nil
	})
	add(srv, "device_open_url", "Open a link or deep link with the app that handles it.", func(ctx context.Context, in struct {
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
	add(srv, "device_apps", "List installed apps; filter by name or package.", func(ctx context.Context, in struct {
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
	add(srv, "device_scroll", "Scroll the content up, down, left or right.", func(ctx context.Context, in struct {
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
	act("device_long_press", "Press and hold an element or a point, e.g. to open a context menu.", func(ctx context.Context, serial string, in tapInput) (string, error) {
		x, y, what, err := m.Resolve(serial, devices.Target{X: in.X, Y: in.Y, Index: in.Index, Text: in.Text, ID: in.ID})
		if err != nil {
			return "", err
		}
		if err := m.LongPress(ctx, serial, x, y); err != nil {
			return "", err
		}
		return fmt.Sprintf("long pressed %d,%d %s", x, y, what), nil
	})
	add(srv, "device_tap_text", "Tap the first element whose text matches, in one call. Prefer it for a known label.", func(ctx context.Context, in struct {
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
	add(srv, "device_find", "Check whether a text is on screen and where.", func(ctx context.Context, in struct {
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
	add(srv, "device_wait_for", "Wait until a text appears on screen (up to 30 s).", func(ctx context.Context, in struct {
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
	add(srv, "device_state", "Read the device state: app in front, screen, battery, dark mode, network.", func(ctx context.Context, in deviceRef) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		return map[string]any{"device": serial, "state": m.Overview(ctx, serial)}, nil
	})
	add(srv, "device_notifications", "List active notifications.", func(ctx context.Context, in deviceRef) (any, error) {
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
	add(srv, "device_screen", "Turn the screen on or off, or unlock one without PIN.", func(ctx context.Context, in struct {
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
	add(srv, "device_setting", "Change and read back a device setting: dark_mode, font_scale, orientation, wifi, data, stay_awake, permission, location.", func(ctx context.Context, in struct {
		deviceRef
		devices.Action
	}) (any, error) {
		serial, err := pick(ctx, in.Serial)
		if err != nil {
			return nil, err
		}
		if in.Action.Name == "state" || in.Action.Name == "" {
			return nil, apperr.New(apperr.Invalid, "pass an action; to read the state use device_state")
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
