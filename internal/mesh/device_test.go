package mesh

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// A phone has no client certificate: it pairs with the code, gets a token and
// a proof bound to the node key, and wrong codes close pairing.
func TestDevicePairing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := start(t, ctx, "a")
	a.m.IssueDevice = func(context.Context, string) (string, error) { return "secret", nil }
	phone := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	url := fmt.Sprintf("https://127.0.0.1:%d/device/v1/pair", a.m.port)
	post := func(code string) *http.Response {
		resp, err := phone.Post(url, "application/json", strings.NewReader(`{"code":"`+code+`","name":"pixel"}`))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	code, _ := a.m.PairCode()
	resp := post(code)
	var out map[string]string
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != 200 || out["token"] != "secret" || out["proof"] != proof(code, a.m.id.DevicePin, "device") {
		t.Fatalf("pair: %d %v", resp.StatusCode, out)
	}
	if resp = post(code); resp.StatusCode != 403 {
		t.Fatalf("a used code must fail, got %d", resp.StatusCode)
	}

	code, _ = a.m.PairCode()
	for range devicePairMaxFails {
		post("0000-0000").Body.Close()
	}
	if resp = post(code); resp.StatusCode != 403 {
		t.Fatalf("pairing must close after wrong codes, got %d", resp.StatusCode)
	}
}
