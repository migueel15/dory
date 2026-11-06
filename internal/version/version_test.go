package version

import (
	"runtime/debug"
	"testing"
)

func TestFromBuildInfoWithoutVCS(t *testing.T) {
	for _, info := range []Info{
		fromBuildInfo(nil, false),
		fromBuildInfo(&debug.BuildInfo{}, true),
	} {
		if info.Revision != "dev" || info.Short != "dev" || info.Time != "" || info.Modified {
			t.Fatalf("got %+v, want dev", info)
		}
	}
}

func TestFromBuildInfoWithVCS(t *testing.T) {
	info := fromBuildInfo(&debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "ae2f2410123456789"},
		{Key: "vcs.time", Value: "2026-11-06T10:00:00Z"},
		{Key: "vcs.modified", Value: "true"},
	}}, true)
	if info.Short != "ae2f241" || info.Revision != "ae2f2410123456789" || info.Time != "2026-11-06T10:00:00Z" || !info.Modified {
		t.Fatalf("got %+v", info)
	}
}
