//go:build linux

package runtime

import (
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// A service started like StartService (sh wrapper, own session, marked) stops.
func TestStopServiceWithWrapperShell(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 missing")
	}
	thread := "test-stop-" + time.Now().Format("150405.000")
	cmd := exec.Command("/bin/sh", "-c", "python3 -m http.server 18765 --bind 127.0.0.1; true")
	cmd.Env = append(threadEnv(os.Environ(), thread), serviceMarker)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait()
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(100 * time.Millisecond)
		for _, s := range ThreadServices(thread) {
			pid = s.PID
		}
	}
	if pid == 0 {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		t.Fatal("service not listed")
	}
	if err := StopService(thread, pid); err != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if len(ThreadServices(thread)) != 0 {
		t.Fatal("still listed")
	}
	if c, err := net.Dial("tcp", "127.0.0.1:18765"); err == nil {
		c.Close()
		t.Fatal("port still open")
	}
}
