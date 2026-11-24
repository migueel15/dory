package mesh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/migueel15/dory/internal/apperr"
	"github.com/migueel15/dory/internal/mesh/peerapi"
	"github.com/migueel15/dory/internal/runtime"
	"github.com/migueel15/dory/internal/update"
)

// ErrUnreachable is returned when the mesh cannot reach a peer.
var ErrUnreachable = apperr.New(apperr.Unreachable, "the environment is not reachable")

// Client calls the APIs of paired nodes.
type Client struct{ mesh *Mesh }

func (m *Mesh) Client() *Client { return &Client{mesh: m} }

// Call exchanges JSON with the peer's API over its authenticated transport.
func (c *Client) Call(ctx context.Context, node, method, path string, in, out any) error {
	if c == nil || c.mesh == nil {
		return unreachable(nil)
	}
	peer, err := c.mesh.peer(ctx, node)
	if err != nil || peer.address == "" {
		return unreachable(err)
	}
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+peer.address+proxyPrefix+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := peer.client().Do(req)
	if err != nil {
		return unreachable(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil {
			return unreachable(err)
		}
		var envelope apperr.Body
		if json.Unmarshal(raw, &envelope) == nil && envelope.Error.Code != "" {
			return &apperr.Error{Code: envelope.Error.Code, Message: envelope.Error.Message, Params: envelope.Error.Params}
		}
		return apperr.Newf(apperr.Unreachable, "the environment answered %d", resp.StatusCode)
	}
	if out == nil {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			return unreachable(err)
		}
		return nil
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return unreachable(err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("respuesta no válida: %w", err)
	}
	return nil
}

func unreachable(cause error) error {
	return apperr.Wrap(ErrUnreachable.Code, cause, ErrUnreachable.Message)
}

func (c *Client) CreateThread(ctx context.Context, node string, in peerapi.CreateThreadRequest) (peerapi.CreateThreadResponse, error) {
	var out peerapi.CreateThreadResponse
	err := c.Call(ctx, node, http.MethodPost, "/api/threads", in, &out)
	return out, err
}

func (c *Client) SendMessage(ctx context.Context, node, thread string, in peerapi.SendMessageRequest) error {
	return c.Call(ctx, node, http.MethodPost, "/api/threads/"+url.PathEscape(thread)+"/messages", in, nil)
}

func (c *Client) InterruptThread(ctx context.Context, node, thread string) error {
	return c.Call(ctx, node, http.MethodPost, "/api/threads/"+url.PathEscape(thread)+"/interrupt", nil, nil)
}

func (c *Client) CompleteDelegation(ctx context.Context, node string, outcome runtime.Outcome) error {
	return c.Call(ctx, node, http.MethodPost, "/api/threads/"+url.PathEscape(outcome.ParentThreadID)+"/delegations/complete", outcome, nil)
}

func (c *Client) UpdateStatus(ctx context.Context, node string, fresh bool) (update.Status, error) {
	var out update.Status
	path := "/api/nodes/" + url.PathEscape(node) + "/update"
	if fresh {
		path += "?fresh=1"
	}
	err := c.Call(ctx, node, http.MethodGet, path, nil, &out)
	return out, err
}

func (c *Client) StartUpdate(ctx context.Context, node string, opts update.Options) (update.Status, error) {
	var out update.Status
	err := c.Call(ctx, node, http.MethodPost, "/api/nodes/"+url.PathEscape(node)+"/update", opts, &out)
	return out, err
}
