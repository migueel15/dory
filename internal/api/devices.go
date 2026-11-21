package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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
	mux.HandleFunc("POST /api/nodes/{node}/devices/avd/{name}/start", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		s.deviceDone(w, nil, s.Devices.StartAVD(r.PathValue("name")))
	}))
	mux.HandleFunc("POST /api/nodes/{node}/devices/{serial}/stop", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		serial, _ := url.PathUnescape(r.PathValue("serial"))
		var body struct {
			Confirm bool `json:"confirm"`
		}
		if r.ContentLength > 0 && !decode(w, r, &body) {
			return
		}
		// Powering off a real phone is not undone from here: the interface asks first.
		if !strings.HasPrefix(serial, "emulator-") && !body.Confirm {
			writeError(w, http.StatusBadRequest, "confirm_required", "apagar un móvil físico necesita confirmación")
			return
		}
		s.deviceDone(w, nil, s.Devices.Stop(r.Context(), serial))
	}))
	mux.HandleFunc("POST /api/nodes/{node}/devices/pair", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Address string `json:"address"`
			Code    string `json:"code"`
		}
		if !decode(w, r, &body) {
			return
		}
		out, err := s.Devices.Pair(r.Context(), body.Address, body.Code)
		s.deviceDone(w, out, err)
	}))
	mux.HandleFunc("POST /api/nodes/{node}/devices/connect", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Address string `json:"address"`
		}
		if !decode(w, r, &body) {
			return
		}
		out, err := s.Devices.Connect(r.Context(), body.Address)
		s.deviceDone(w, out, err)
	}))
	mux.HandleFunc("POST /api/nodes/{node}/devices/{serial}/action", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		serial, _ := url.PathUnescape(r.PathValue("serial"))
		var a devices.Action
		if !decode(w, r, &a) {
			return
		}
		out, err := s.Devices.Apply(r.Context(), serial, a)
		if err != nil {
			writeError(w, http.StatusBadRequest, "device_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	}))
	mux.HandleFunc("POST /api/nodes/{node}/devices/{serial}/disconnect", s.onNode(func(w http.ResponseWriter, r *http.Request) {
		serial, _ := url.PathUnescape(r.PathValue("serial"))
		s.deviceDone(w, nil, s.Devices.Disconnect(r.Context(), serial))
	}))
}

func (s *Server) deviceDone(w http.ResponseWriter, out any, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, "device_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": out})
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	m := s.Devices
	out := map[string]any{"tools": m.Tools, "devices": []deviceView{}, "avds": devices.ListAVDs(r.Context(), m.Tools), "avd_states": m.AVDStates()}
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
