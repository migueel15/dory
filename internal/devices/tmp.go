package devices

import (
	"os"
	"sync"
)

var (
	tmpOnce sync.Once
	tmpPath string
	tmpErr  error
)

// writeTemp stores the embedded jar once per process so adb can push it.
func writeTemp(b []byte) (string, error) {
	tmpOnce.Do(func() {
		f, err := os.CreateTemp("", "dory-scrcpy-*.jar")
		if err != nil {
			tmpErr = err
			return
		}
		defer f.Close()
		_, tmpErr = f.Write(b)
		tmpPath = f.Name()
	})
	return tmpPath, tmpErr
}
