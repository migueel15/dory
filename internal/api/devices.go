package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/migueel15/dory/internal/devices"
)

// deviceView is a device as the interface shows it.
type deviceView struct {
	devices.Device
	Streaming bool `json:"streaming"`
}

func (s *Server) mountDevices(mux *http.ServeMux) {
	if s.Devices == nil {
		return
	}
	mux.HandleFunc("GET /api/nodes/{node}/devices", s.onNode(s.listDevices))
	mux.HandleFunc("GET /api/nodes/{node}/devices/{serial}/stream", s.onNode(s.streamDevice))
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	m := s.Devices
	out := map[string]any{"tools": m.Tools, "devices": []deviceView{}, "avds": devices.ListAVDs(r.Context(), m.Tools)}
	if m.Tools.ADB != "" {
		list, err := m.ADB.Devices(r.Context())
		if err != nil {
			out["error"] = err.Error()
		}
		views := []deviceView{}
		for _, d := range list {
			if d.Transport == "emulator" && d.State == "device" {
				d.AVD = m.ADB.AVDName(r.Context(), d.Serial)
			}
			views = append(views, deviceView{Device: d, Streaming: m.Active(d.Serial)})
		}
		out["devices"] = views
	}
	writeJSON(w, http.StatusOK, out)
}

// streamDevice sends the device's H.264 video as binary messages (a kind
// byte, then the NAL data) after a JSON "meta" message, and injects the
// scrcpy control messages the viewer sends back.
func (s *Server) streamDevice(w http.ResponseWriter, r *http.Request) {
	serial, _ := url.PathUnescape(r.PathValue("serial"))
	if serial == "" || !sameSite(r) {
		writeError(w, http.StatusForbidden, "forbidden", "origen no permitido")
		return
	}
	if s.Devices.Tools.ADB == "" {
		writeError(w, http.StatusFailedDependency, "no_adb", devices.ErrNoADB.Error())
		return
	}
	q := r.URL.Query()
	opts := devices.Options{}
	opts.MaxSize, _ = strconv.Atoi(q.Get("max_size"))
	opts.Bitrate, _ = strconv.Atoi(q.Get("bitrate"))
	opts.MaxSize = min(opts.MaxSize, 2560)
	opts.Bitrate = min(opts.Bitrate, 20_000_000)
	ws, err := upgradeWS(w, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_argument", err.Error())
		return
	}
	defer ws.close()
	sub, meta, err := s.Devices.Subscribe(r.Context(), serial, opts)
	if err != nil {
		raw, _ := json.Marshal(map[string]any{"type": "error", "message": err.Error()})
		_ = ws.write(wsText, raw)
		return
	}
	defer sub.Leave()
	raw, _ := json.Marshal(map[string]any{"type": "meta", "name": meta.Name, "codec": meta.Codec, "width": meta.Width, "height": meta.Height})
	if ws.write(wsText, raw) != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			op, msg, err := ws.read(1 << 16)
			if err != nil {
				return
			}
			if op == wsBinary {
				_ = s.Devices.Control(serial, msg)
			}
		}
	}()
	for {
		select {
		case frame, ok := <-sub.C:
			if !ok {
				raw, _ := json.Marshal(map[string]any{"type": "closed"})
				_ = ws.write(wsText, raw)
				return
			}
			if ws.write(wsBinary, frame) != nil {
				return
			}
		case <-done:
			return
		}
	}
}
