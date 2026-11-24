package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/migueel15/dory/internal/data"
)

// checkProtocol records the protocol before exchanging any operations.
func (m *Mesh) checkProtocol(ctx context.Context, c *http.Client, base, node, address, key string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/info", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var info json.RawMessage
	var metadata struct {
		Protocol int                `json:"protocol"`
		Threads  []data.ThreadState `json:"threads"`
	}
	switch {
	case resp.StatusCode == http.StatusNotFound,
		resp.StatusCode == http.StatusOK && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html"):
		// Older nodes can serve their web shell for unrecognized mesh paths.
		metadata.Protocol = 1
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("%s: %s", req.URL.Path, resp.Status)
	default:
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return false, err
		}
		if err := json.Unmarshal(info, &metadata); err != nil {
			return false, err
		}
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return false, err
	}
	const currentPeer = ` WHERE node_id = ? AND address = ? AND public_key = ? AND revoked_at IS NULL AND disabled_at IS NULL`
	if metadata.Protocol != ProtocolVersion {
		message := fmt.Sprintf("%s: local=%d remote=%d", incompatibleProtocol, ProtocolVersion, metadata.Protocol)
		_, err := m.db.ExecContext(ctx, `UPDATE peers SET protocol = ?, last_error = ?`+currentPeer, metadata.Protocol, message, node, address, key)
		return false, err
	}
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `UPDATE peers SET protocol = ?, last_error = '', last_seen = ?, info = json_remove(?, '$.threads')`+currentPeer,
		metadata.Protocol, at, string(info), node, address, key)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return false, err
	}
	changed, err := m.cacheThreadStates(ctx, tx, node, at, metadata.Threads)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if m.OnThreadState != nil {
		for _, id := range changed {
			m.OnThreadState(id)
		}
	}
	return true, nil
}
