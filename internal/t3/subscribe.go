package t3

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/coder/websocket"
)

// Streams Dory follows. Their chunks are only used as a signal that something
// changed: the data is re-read through the stable calls, so a change in the
// shape of T3's stream messages cannot corrupt what Dory shows.
const (
	methodSubscribeShell  = "orchestration.subscribeShell"
	methodSubscribeThread = "orchestration.subscribeThread"
)

var errStreamEnded = errors.New("t3: stream ended")

// shellPayload matches what the T3 web client asks for: the same threads the
// HTTP shell read reports.
var shellPayload = map[string]any{"omitIdleSubagents": true, "omitOlderSettled": true}

// Stream runs a streaming RPC and calls onChunk after each chunk (the first
// one is the snapshot). It returns when ctx ends or the stream fails. Unlike
// call, the connection stays open as long as the caller needs it.
func (c Client) Stream(ctx context.Context, method string, payload any, onChunk func()) error {
	dial, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	ticket, err := c.ticket(dial)
	if err != nil {
		return err
	}
	target, err := c.wsURL(ticket)
	if err != nil {
		return err
	}
	conn, resp, err := websocket.Dial(dial, target, &websocket.DialOptions{HTTPClient: c.HTTP})
	if err != nil {
		if resp != nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
			return errUnauthorized
		}
		return fmt.Errorf("t3: websocket: %w", err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(rpcReadLimit)
	id := newID()
	req, err := json.Marshal(map[string]any{"_tag": "Request", "id": id, "tag": method, "payload": payload, "headers": []any{}})
	if err != nil {
		return err
	}
	if err := conn.Write(dial, websocket.MessageText, req); err != nil {
		return err
	}
	ack, err := json.Marshal(map[string]string{"_tag": "Ack", "requestId": id})
	if err != nil {
		return err
	}
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("t3: %s: %w", method, err)
		}
		msgs, err := decodeFrame(data)
		if err != nil {
			return err
		}
		for _, m := range msgs {
			switch m.Tag {
			case "Ping":
				err = conn.Write(ctx, websocket.MessageText, []byte(`{"_tag":"Pong"}`))
			case "Chunk":
				// T3 sends the next chunk only after the client acknowledges this one.
				if err = conn.Write(ctx, websocket.MessageText, ack); err == nil {
					onChunk()
				}
			case "Exit":
				return errStreamEnded
			case "Defect", "ClientProtocolError":
				return fmt.Errorf("t3: %s: %s", method, data)
			}
			if err != nil {
				return err
			}
		}
	}
}
