package nodeapp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func TestShellJoinFlow(t *testing.T) {
	host, err := Start(context.Background(), Options{DataDir: t.TempDir(), NoMDNS: true, DeviceName: "desktop", Headless: true,
		Confirm: func(string, string) bool { return true }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Stop)
	_, port, _ := strings.Cut(host.PeerAddr, "]:")
	if port == "" || port == host.PeerAddr {
		_, port, _ = strings.Cut(host.PeerAddr, ":")
	}
	host.Pairing.Addrs = []string{"127.0.0.1:" + port}
	offer, err := host.Pairing.Start(0)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	s, err := StartShell(context.Background(), Options{DataDir: dir, NoMDNS: true, DeviceName: "phone"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	base := "http://" + s.Addr()

	// Without the cookie nothing answers, not even the join page.
	for _, p := range []string{"/", "/node/join.html", "/api/items"} {
		resp, err := http.Get(base + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 403 {
			t.Fatalf("GET %s without cookie: %d", p, resp.StatusCode)
		}
	}
	if resp, _ := http.Get(base + "/__enter?t=wrong"); resp.StatusCode != 403 {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	resp, err := c.Get(s.EnterURL())
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Request.URL.Path, "join.html") || !strings.Contains(string(page), "Unirse a un grupo") {
		t.Fatalf("enter: %d %s", resp.StatusCode, resp.Request.URL)
	}
	if resp, _ := c.Get(base + "/api/items"); resp.StatusCode != 503 {
		t.Fatalf("API before join: %d", resp.StatusCode)
	}

	post := func(body string, hdr bool) (int, string) {
		req, _ := http.NewRequest("POST", base+joinAPI, bytes.NewBufferString(body))
		if hdr {
			req.Header.Set(csrfHeader, "1")
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if code, _ := post(`{"payload":"x"}`, false); code != 403 {
		t.Fatalf("join without CSRF header: %d", code)
	}
	if code, _ := post(`{"payload":"{}","name":"phone"}`, true); code != 502 {
		t.Fatalf("join with bad payload: %d", code)
	}
	b, _ := jsonString(offer.Payload)
	code, body := post(`{"payload":`+b+`,"name":"phone"}`, true)
	if code != 200 {
		t.Fatalf("join: %d %s", code, body)
	}
	if !Joined(dir) || s.Node() == nil {
		t.Fatal("not joined / node not started")
	}
	// Now the node answers behind the same cookie, as the local user.
	resp, err = c.Get(base + "/node/api/state")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(st), "desktop") {
		t.Fatalf("state: %d %s", resp.StatusCode, st)
	}
	// Joining twice is refused.
	if code, _ := post(`{"payload":`+b+`}`, true); code == 200 {
		t.Fatal("second join accepted")
	}

	// Network push from the Java side uses the gate header, not the cookie.
	req, _ := http.NewRequest("POST", base+netAPI, strings.NewReader(`[{"name":"wlan0","index":1,"mtu":1500,"ips":["192.168.1.5"]}]`))
	if r, _ := http.DefaultClient.Do(req); r.StatusCode != 403 {
		t.Fatalf("net without gate: %d", r.StatusCode)
	}
	req, _ = http.NewRequest("POST", base+netAPI, strings.NewReader(`[{"name":"wlan0","index":1,"mtu":1500,"ips":["192.168.1.5"]}]`))
	req.Header.Set(gateHeader, s.Gate())
	if r, err := http.DefaultClient.Do(req); err != nil || r.StatusCode != 200 {
		t.Fatalf("net: %v %v", r, err)
	}
	if got := s.Node().localAddrs(); len(got) != 1 || !strings.HasPrefix(got[0], "192.168.1.5:") {
		t.Fatalf("local addrs: %v", got)
	}
}

func jsonString(s string) (string, error) {
	var b bytes.Buffer
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}
