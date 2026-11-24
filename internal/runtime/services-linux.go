//go:build linux

package runtime

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/harness"
	"golang.org/x/sys/unix"
)

type Service struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
	Port    int    `json:"port"`
	Command string `json:"command"`
	Kind    string `json:"kind"`
}

type procInfo struct {
	pid, group, parent     int
	thread, command, start string
	// service marks processes started by StartService (DORY_SERVICE=1), so
	// their wrapper shell and Dory as parent do not block a stop.
	service bool
}

var ErrSharedProcessGroup = apperr.New(apperr.SharedProcessGroup, "the service shares a process group with Dory, the agent or a shell")

// safeProcessGroup examines every process, not only those with a thread tag.
// A shell or provider in the same group must never receive a service signal.
func (m *Manager) safeProcessGroup(group int, thread string) bool {
	if group <= 1 || group == syscall.Getpgrp() {
		return false
	}
	drivers := m.registry.All()
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	marked := processes()
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		end := strings.LastIndexByte(string(stat), ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(stat[end+1:]))
		if len(fields) < 3 {
			continue
		}
		pgid, err := strconv.Atoi(fields[2])
		if err != nil || pgid != group {
			continue
		}
		if pid == os.Getpid() {
			return false
		}
		p, ok := marked[pid]
		if !ok || p.thread != thread || p.group != group {
			return false
		}
		if p.service {
			continue
		}
		if p.parent == os.Getpid() {
			return false
		}
		parentName, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", p.parent))
		if err == nil && strings.TrimSpace(string(parentName)) == "systemd-run" {
			return false
		}
		name := strings.ToLower(string(stat[strings.IndexByte(string(stat), '(')+1 : end]))
		switch name {
		case "sh", "bash", "zsh", "fish", "dash", "ksh", "systemd-run", "dory":
			return false
		}
		for _, d := range drivers {
			if slices.Contains(d.ProcessNames, name) {
				return false
			}
		}
	}
	return true
}

func processes() map[int]procInfo {
	entries, _ := os.ReadDir("/proc")
	result := map[int]procInfo{}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 {
			continue
		}
		base := filepath.Join("/proc", e.Name())
		env, err := os.ReadFile(filepath.Join(base, "environ"))
		if err != nil {
			continue
		}
		id, service := "", false
		for _, v := range strings.Split(string(env), "\x00") {
			if strings.HasPrefix(v, "DORY_THREAD_ID=") {
				id = strings.TrimPrefix(v, "DORY_THREAD_ID=")
			}
			if v == serviceMarker {
				service = true
			}
		}
		if id == "" {
			continue
		}
		stat, err := os.ReadFile(filepath.Join(base, "stat"))
		if err != nil {
			continue
		}
		end := strings.LastIndexByte(string(stat), ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(stat[end+1:]))
		if len(fields) <= 19 {
			continue
		}
		cmd, _ := os.ReadFile(filepath.Join(base, "cmdline"))
		command := strings.TrimSpace(strings.ReplaceAll(string(cmd), "\x00", " "))
		group, _ := strconv.Atoi(fields[2])
		parent, _ := strconv.Atoi(fields[1])
		result[pid] = procInfo{pid: pid, group: group, parent: parent, thread: id, command: command, start: fields[19], service: service}
	}
	return result
}

type socketInfo struct {
	address string
	port    int
}

func sockets() map[string]socketInfo {
	out := map[string]socketInfo{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 || fields[3] != "0A" {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			b, err := hex.DecodeString(parts[0])
			if err != nil || (len(b) != 4 && len(b) != 16) {
				continue
			}
			for i := 0; i < len(b); i += 4 {
				b[i], b[i+3] = b[i+3], b[i]
				b[i+1], b[i+2] = b[i+2], b[i+1]
			}
			port, err := strconv.ParseUint(parts[1], 16, 16)
			if err != nil {
				continue
			}
			out[fields[9]] = socketInfo{net.IP(b).String(), int(port)}
		}
		f.Close()
	}
	return out
}

func serviceKind(cmd string, port int) string {
	c := strings.ToLower(cmd)
	for _, x := range []struct{ match, kind string }{{"postgres", "postgresql"}, {"redis-server", "redis"}, {"mysqld", "mysql"}, {"mongod", "mongodb"}, {"vite", "vite"}, {"next", "next"}, {"http.server", "http"}, {"uvicorn", "http"}, {"docker-proxy", "docker"}} {
		if strings.Contains(c, x.match) {
			return x.kind
		}
	}
	switch port {
	case 5432:
		return "postgresql"
	case 6379:
		return "redis"
	case 3306:
		return "mysql"
	case 27017:
		return "mongodb"
	case 5173:
		return "vite"
	}
	if strings.Contains(c, "node") {
		return "node"
	}
	return "process"
}

func (m *Manager) ThreadServices(id string) []Service {
	drivers := m.registry.All()
	sockets := sockets()
	out := []Service{}
	for _, p := range processes() {
		if p.thread != id || p.pid == os.Getpid() || (p.parent == os.Getpid() && !p.service) {
			continue
		}
		parentName, _ := os.ReadFile(fmt.Sprintf("/proc/%d/comm", p.parent))
		if strings.TrimSpace(string(parentName)) == "systemd-run" {
			continue
		}
		// The provider's own control endpoint is not a service started by it.
		if slices.ContainsFunc(drivers, func(d harness.EntryPoint) bool { return d.ControlProcess != nil && d.ControlProcess(p.command) }) {
			continue
		}
		fds, _ := os.ReadDir(fmt.Sprintf("/proc/%d/fd", p.pid))
		for _, fd := range fds {
			target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", p.pid, fd.Name()))
			if err != nil || !strings.HasPrefix(target, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			s, ok := sockets[inode]
			if !ok {
				continue
			}
			out = append(out, Service{p.pid, s.address, s.port, p.command, serviceKind(p.command, s.port)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].PID != out[j].PID {
			return out[i].PID < out[j].PID
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// StopService checks ownership and process identity again before each signal.
// A listener sharing a process group with an agent or shell cannot be stopped safely.
func (m *Manager) StopService(id string, pid int) error {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	p, ok := processes()[pid]
	if !ok || p.thread != id {
		return ErrNotFound
	}
	listening := false
	for _, s := range m.ThreadServices(id) {
		if s.PID == pid {
			listening = true
			break
		}
	}
	if !listening {
		return ErrNotFound
	}
	if !m.safeProcessGroup(p.group, id) {
		return ErrSharedProcessGroup
	}
	type member struct {
		fd int
		p  procInfo
	}
	var members []member
	{
		for child, info := range processes() {
			if child == pid || info.thread != id || info.group != p.group {
				continue
			}
			childFD, err := unix.PidfdOpen(child, 0)
			if err != nil {
				continue
			}
			current, ok := processes()[child]
			if !ok || current.start != info.start || current.thread != id || current.group != p.group {
				unix.Close(childFD)
				continue
			}
			members = append(members, member{childFD, info})
		}
	}
	defer func() {
		for _, m := range members {
			unix.Close(m.fd)
		}
	}()
	for _, m := range members {
		_ = unix.PidfdSendSignal(m.fd, syscall.SIGTERM, nil, 0)
	}
	if err := unix.PidfdSendSignal(fd, syscall.SIGTERM, nil, 0); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		now, ok := processes()[pid]
		if (!ok || now.start != p.start || now.thread != id) && len(members) == 0 {
			break
		}
	}
	for _, m := range members {
		_ = unix.PidfdSendSignal(m.fd, syscall.SIGKILL, nil, 0)
	}
	now, ok := processes()[pid]
	if !ok || now.start != p.start || now.thread != id {
		return nil
	}
	return unix.PidfdSendSignal(fd, syscall.SIGKILL, nil, 0)
}

func StopThreadServices(id string) {
	type target struct {
		fd      int
		process procInfo
	}
	var targets []target
	for pid, p := range processes() {
		if p.thread != id || pid == os.Getpid() {
			continue
		}
		fd, err := unix.PidfdOpen(pid, 0)
		if err != nil {
			continue
		}
		current, ok := processes()[pid]
		if !ok || current.start != p.start || current.thread != id {
			unix.Close(fd)
			continue
		}
		if unix.PidfdSendSignal(fd, syscall.SIGTERM, nil, 0) != nil {
			unix.Close(fd)
			continue
		}
		targets = append(targets, target{fd, p})
	}
	if len(targets) > 0 {
		time.Sleep(3 * time.Second)
	}
	current := processes()
	for _, t := range targets {
		p, ok := current[t.process.pid]
		if ok && p.start == t.process.start && p.thread == id {
			_ = unix.PidfdSendSignal(t.fd, syscall.SIGKILL, nil, 0)
		}
		unix.Close(t.fd)
	}
}

func (m *Manager) StopAllServices() {
	ids := map[string]bool{}
	for _, p := range processes() {
		ids[p.thread] = true
	}
	for id := range ids {
		var exists int
		if m.db.QueryRow("SELECT 1 FROM threads WHERE id = ? AND node_id = ?", id, m.node).Scan(&exists) == nil {
			StopThreadServices(id)
		}
	}
}

func (m *Manager) StopArchivedServices() {
	ids := map[string]bool{}
	for _, p := range processes() {
		ids[p.thread] = true
	}
	for id := range ids {
		var exists int
		if m.db.QueryRow("SELECT 1 FROM threads WHERE id = ? AND node_id = ? AND archived_at IS NOT NULL", id, m.node).Scan(&exists) == nil {
			StopThreadServices(id)
		}
	}
}
