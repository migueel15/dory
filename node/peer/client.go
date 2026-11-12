package peer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/migueel15/dory/node/syncx"
)

// Client is a syncx.Transport (and Snapshotter) over the peer HTTP API.
type Client struct {
	// Addr is host:port of the peer.
	Addr string
	// WantDevice, if set, is the device that must answer (pinned by key).
	WantDevice string

	http   *http.Client // bounded requests
	stream *http.Client // long-lived subscription
	iv     Intervals
	pres   *Presence
}

// NewClient dials addr as id. pres may be nil; when set, the subscription
// reports links and heartbeats of wantDevice to it.
func NewClient(id *Identity, m Membership, addr, wantDevice string, iv Intervals, pres *Presence) (*Client, error) {
	cfg, err := id.ClientTLS(m, wantDevice)
	if err != nil {
		return nil, err
	}
	cfg.NextProtos = []string{"http/1.1"}
	mk := func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout, Transport: &http.Transport{
			TLSClientConfig: cfg.Clone(), DisableKeepAlives: true,
			TLSHandshakeTimeout: 10 * time.Second,
		}}
	}
	return &Client{Addr: addr, WantDevice: wantDevice, http: mk(60 * time.Second), stream: mk(0), iv: iv, pres: pres}, nil
}

func (c *Client) get(ctx context.Context, hc *http.Client, path string, q url.Values) (*http.Response, error) {
	u := "https://" + c.Addr + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("peer: %s: %s %s", path, resp.Status, strings.TrimSpace(string(b)))
	}
	return resp, nil
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	resp, err := c.get(ctx, c.http, path, q)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// Vector implements syncx.Transport.
func (c *Client) Vector(ctx context.Context) (syncx.VersionVector, error) {
	vv := syncx.VersionVector{}
	return vv, c.getJSON(ctx, PathVector, nil, &vv)
}

// Pull implements syncx.Transport.
func (c *Client) Pull(ctx context.Context, since syncx.VersionVector, limit int) ([]syncx.Op, error) {
	var ops []syncx.Op
	err := c.getJSON(ctx, PathOps, url.Values{"since": {FormatVector(since)}, "limit": {strconv.Itoa(limit)}}, &ops)
	return ops, err
}

// Snapshot implements syncx.Snapshotter.
func (c *Client) Snapshot(ctx context.Context) (syncx.VersionVector, []syncx.Op, error) {
	var b SnapshotBody
	err := c.getJSON(ctx, PathSnapshot, nil, &b)
	return b.Vector, b.Ops, err
}

// Presence asks the peer for its own view.
func (c *Client) Presence(ctx context.Context) (PresenceBody, error) {
	var b PresenceBody
	return b, c.getJSON(ctx, PathPresence, nil, &b)
}

// Subscribe implements syncx.Transport. The channel closes when ctx ends, the
// link drops or no event (heartbeat included) arrives within the timeout.
func (c *Client) Subscribe(ctx context.Context, since syncx.VersionVector) (<-chan []syncx.Op, error) {
	ctx, cancel := context.WithCancel(ctx)
	resp, err := c.get(ctx, c.stream, PathStream, url.Values{"since": {FormatVector(since)}})
	if err != nil {
		cancel()
		return nil, err
	}
	if c.pres != nil && c.WantDevice != "" {
		c.pres.Up(c.WantDevice)
	}
	out := make(chan []syncx.Op)
	events := make(chan sseEvent)
	go func() { // reader
		defer close(events)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 64<<20)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if ev.name != "" {
					select {
					case events <- ev:
					case <-ctx.Done():
						return
					}
				}
				ev = sseEvent{}
			case strings.HasPrefix(line, "event: "):
				ev.name = line[7:]
			case strings.HasPrefix(line, "data: "):
				ev.data = line[6:]
			}
		}
	}()
	go func() {
		defer close(out)
		defer cancel()
		defer resp.Body.Close()
		if c.pres != nil && c.WantDevice != "" {
			defer c.pres.Down(c.WantDevice)
		}
		dead := time.NewTimer(c.iv.timeout())
		defer dead.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-dead.C:
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if !dead.Stop() {
					select {
					case <-dead.C:
					default:
					}
				}
				dead.Reset(c.iv.timeout())
				if c.pres != nil && c.WantDevice != "" {
					c.pres.Beat(c.WantDevice)
				}
				if ev.name != "ops" {
					continue
				}
				var ops []syncx.Op
				if json.Unmarshal([]byte(ev.data), &ops) != nil {
					return
				}
				select {
				case out <- ops:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

type sseEvent struct{ name, data string }

var (
	_ syncx.Transport   = (*Client)(nil)
	_ syncx.Snapshotter = (*Client)(nil)
)
