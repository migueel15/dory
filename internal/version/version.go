// Package version reports the VCS revision Go embeds in the binary.
package version

import (
	"encoding/json"
	"net/http"
	"runtime/debug"
)

// Info describes the build of the running binary.
type Info struct {
	Revision string `json:"revision"`
	Short    string `json:"short"`
	Time     string `json:"time"`
	Modified bool   `json:"modified"`
}

// fromBuildInfo derives Info from build metadata; without VCS data it is "dev".
func fromBuildInfo(bi *debug.BuildInfo, ok bool) Info {
	info := Info{Revision: "dev", Short: "dev"}
	if !ok || bi == nil {
		return info
	}
	var revision string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			info.Time = s.Value
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}
	if revision == "" {
		info.Time, info.Modified = "", false
		return info
	}
	info.Revision = revision
	info.Short = revision
	if len(revision) > 7 {
		info.Short = revision[:7]
	}
	return info
}

// Read returns the version of the running binary.
func Read() Info { return fromBuildInfo(debug.ReadBuildInfo()) }

// Handler serves GET /api/version.
func Handler() http.HandlerFunc {
	info := Read()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
	}
}
