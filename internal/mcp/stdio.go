package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// RelayStdio connects agents that only support stdio MCP servers to Dory's
// stateless JSON HTTP endpoint. The token stays in the child environment.
func RelayStdio(ctx context.Context, endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.Path != "/mcp" || u.User != nil {
		return errors.New("invalid Dory MCP endpoint")
	}
	switch u.Hostname() {
	case "127.0.0.1", "::1", "localhost":
	default:
		return errors.New("Dory MCP relay requires a loopback endpoint")
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	token := os.Getenv("DORY_MCP_TOKEN")
	scan := bufio.NewScanner(os.Stdin)
	scan.Buffer(make([]byte, 64<<10), 32<<20)
	for scan.Scan() {
		line := scan.Bytes()
		var frame struct{ ID json.RawMessage }
		if json.Unmarshal(line, &frame) != nil {
			continue
		}
		out, err := relayMCP(ctx, client, endpoint, token, line)
		if err != nil {
			if len(frame.ID) != 0 {
				out, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": frame.ID, "error": map[string]any{"code": -32603, "message": err.Error()}})
			}
		}
		if len(out) > 0 {
			if _, err := fmt.Fprintln(os.Stdout, string(out)); err != nil {
				return err
			}
		}
	}
	return scan.Err()
}

func relayMCP(ctx context.Context, client *http.Client, endpoint, token string, line []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(line))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Dory MCP: HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return bytes.TrimSpace(raw), err
}
