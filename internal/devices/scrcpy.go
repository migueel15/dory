package devices

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"os/exec"
	"strings"
	"time"
)

// ScrcpyVersion is the version of the embedded server; its protocol is the one spoken here.
const ScrcpyVersion = "3.3.1"

//go:embed assets/scrcpy-server-v3.3.1.jar
var serverJar []byte

const remoteJar = "/data/local/tmp/dory-scrcpy-server.jar"

// Control message types of scrcpy's protocol.
const (
	CtlKeycode     = 0
	CtlText        = 1
	CtlTouch       = 2
	CtlScroll      = 3
	CtlBackOrOn    = 4
	CtlDisplayPwr  = 10
	CtlResetVideo  = 17
	maxControlSize = 1 << 14
)

// Meta is the header scrcpy sends before the video.
type Meta struct {
	Name   string `json:"name"`
	Codec  string `json:"codec"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Packet is one video packet.
type Packet struct {
	PTS    int64
	Config bool
	Key    bool
	Data   []byte
}

// ReadMeta reads the device name (64 bytes), then the codec id, width and height.
func ReadMeta(r io.Reader) (Meta, error) {
	var name [64]byte
	if _, err := io.ReadFull(r, name[:]); err != nil {
		return Meta{}, err
	}
	var h [12]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return Meta{}, err
	}
	return Meta{Name: strings.TrimRight(string(name[:]), "\x00"), Codec: string(h[:4]), Width: int(binary.BigEndian.Uint32(h[4:8])), Height: int(binary.BigEndian.Uint32(h[8:12]))}, nil
}

// ReadPacket reads a frame header (flags+pts, size) and its data.
func ReadPacket(r io.Reader) (Packet, error) {
	var h [12]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return Packet{}, err
	}
	v := binary.BigEndian.Uint64(h[:8])
	size := binary.BigEndian.Uint32(h[8:])
	if size > 16<<20 {
		return Packet{}, errors.New("paquete de vídeo demasiado grande")
	}
	p := Packet{Config: v>>63 == 1, Key: v>>62&1 == 1, PTS: int64(v & (1<<62 - 1)), Data: make([]byte, size)}
	_, err := io.ReadFull(r, p.Data)
	return p, err
}

// Options tune the stream.
type Options struct {
	MaxSize int
	Bitrate int
}

// normalized fills the defaults and keeps the values in a sane range; a
// MaxSize of 4096 or more leaves the screen at its native size.
func (o Options) normalized() Options {
	if o.MaxSize <= 0 {
		o.MaxSize = 1080
	}
	if o.Bitrate <= 0 {
		o.Bitrate = 6_000_000
	}
	o.MaxSize = min(o.MaxSize, 4096)
	o.Bitrate = min(o.Bitrate, 40_000_000)
	return o
}

type scrcpyConn struct {
	meta    Meta
	video   net.Conn
	control net.Conn
	proc    *exec.Cmd
	port    int
	serial  string
	adb     ADB
}

func (c *scrcpyConn) close() {
	if c.video != nil {
		c.video.Close()
	}
	if c.control != nil {
		c.control.Close()
	}
	if c.proc != nil && c.proc.Process != nil {
		_ = c.proc.Process.Kill()
		_ = c.proc.Wait()
	}
	if c.port != 0 {
		_, _ = c.adb.Run(context.Background(), 4*time.Second, "-s", c.serial, "forward", "--remove", fmt.Sprintf("tcp:%d", c.port))
	}
}

// startScrcpy pushes the server, starts it and opens the video and control sockets.
func startScrcpy(ctx context.Context, a ADB, serial string, o Options) (*scrcpyConn, error) {
	o = o.normalized()
	tmp, err := writeTemp(serverJar)
	if err != nil {
		return nil, err
	}
	if _, err := a.Run(ctx, 20*time.Second, "-s", serial, "push", tmp, remoteJar); err != nil {
		return nil, err
	}
	scid := fmt.Sprintf("%08x", rand.Uint32()&0x7fffffff)
	out, err := a.Run(ctx, 5*time.Second, "-s", serial, "forward", "tcp:0", "localabstract:scrcpy_"+scid)
	if err != nil {
		return nil, err
	}
	var port int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &port)
	if port == 0 {
		return nil, fmt.Errorf("adb forward no devolvió un puerto: %q", out)
	}
	c := &scrcpyConn{serial: serial, adb: a, port: port}
	args := []string{"-s", serial, "shell", "CLASSPATH=" + remoteJar, "app_process", "/", "com.genymobile.scrcpy.Server", ScrcpyVersion,
		"scid=" + scid, "log_level=info", "video=true", "audio=false", "control=true", "tunnel_forward=true",
		"send_device_meta=true", "send_frame_meta=true", "send_dummy_byte=true", "send_codec_meta=true", "video_codec=h264",
		fmt.Sprintf("max_size=%d", o.MaxSize), fmt.Sprintf("video_bit_rate=%d", o.Bitrate), "cleanup=false"}
	c.proc = exec.Command(a.Path, args...)
	if err := c.proc.Start(); err != nil {
		c.close()
		return nil, err
	}
	// The server needs a moment to listen; retry the video socket.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(8 * time.Second)
	for {
		conn, derr := net.DialTimeout("tcp", addr, time.Second)
		if derr == nil {
			// With a forward tunnel the server writes one dummy byte once it accepted us.
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			var b [1]byte
			if _, rerr := io.ReadFull(conn, b[:]); rerr == nil {
				conn.SetReadDeadline(time.Time{})
				c.video = conn
				break
			}
			conn.Close()
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			c.close()
			return nil, errors.New("scrcpy no respondió en el dispositivo")
		}
		time.Sleep(150 * time.Millisecond)
	}
	if c.control, err = net.DialTimeout("tcp", addr, 3*time.Second); err != nil {
		c.close()
		return nil, err
	}
	c.video.SetReadDeadline(time.Now().Add(10 * time.Second))
	if c.meta, err = ReadMeta(c.video); err != nil {
		c.close()
		return nil, fmt.Errorf("cabecera de scrcpy: %w", err)
	}
	c.video.SetReadDeadline(time.Time{})
	return c, nil
}
