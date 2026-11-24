package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// wsConn is a minimal RFC 6455 server connection: text and binary messages,
// ping/pong and close. It avoids a dependency for the one stream that needs it.
type wsConn struct {
	c   net.Conn
	br  *bufio.Reader
	wmu sync.Mutex
}

const (
	wsText   = 1
	wsBinary = 2
	wsClose  = 8
	wsPing   = 9
	wsPong   = 10
)

// sameSite accepts requests with no Origin or whose Origin points at this host.
func sameSite(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	u, err := url.Parse(o)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

func upgradeWS(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, errors.New("expected a WebSocket connection")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	hj, ok := w.(http.Hijacker)
	if key == "" || !ok {
		return nil, errors.New("WebSocket not available")
	}
	c, rw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
	if err == nil {
		err = rw.Flush()
	}
	if err != nil {
		c.Close()
		return nil, err
	}
	return &wsConn{c: c, br: rw.Reader}, nil
}

func (w *wsConn) close() { w.c.Close() }

func (w *wsConn) write(op byte, payload []byte) error {
	w.wmu.Lock()
	defer w.wmu.Unlock()
	h := []byte{0x80 | op}
	switch n := len(payload); {
	case n < 126:
		h = append(h, byte(n))
	case n < 1<<16:
		h = append(h, 126, byte(n>>8), byte(n))
	default:
		h = append(h, 127)
		h = binary.BigEndian.AppendUint64(h, uint64(n))
	}
	w.c.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if _, err := w.c.Write(h); err != nil {
		return err
	}
	_, err := w.c.Write(payload)
	return err
}

// read returns the next data message, answering pings and ending on close.
func (w *wsConn) read(limit int) (byte, []byte, error) {
	var msg []byte
	var first byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(w.br, h[:]); err != nil {
			return 0, nil, err
		}
		fin, op := h[0]&0x80 != 0, h[0]&0x0f
		n := uint64(h[1] & 0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return 0, nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(w.br, b[:]); err != nil {
				return 0, nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if h[1]&0x80 == 0 || n > uint64(limit) {
			return 0, nil, errors.New("invalid frame")
		}
		var mask [4]byte
		if _, err := io.ReadFull(w.br, mask[:]); err != nil {
			return 0, nil, err
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(w.br, p); err != nil {
			return 0, nil, err
		}
		for i := range p {
			p[i] ^= mask[i%4]
		}
		switch op {
		case wsPing:
			_ = w.write(wsPong, p)
			continue
		case wsPong:
			continue
		case wsClose:
			_ = w.write(wsClose, nil)
			return 0, nil, io.EOF
		case wsText, wsBinary:
			first, msg = op, p
		case 0:
			msg = append(msg, p...)
			if len(msg) > limit {
				return 0, nil, errors.New("mensaje demasiado grande")
			}
		}
		if fin {
			return first, msg, nil
		}
	}
}
