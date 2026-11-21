package devices

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestParseDevices(t *testing.T) {
	out := `List of devices attached
emulator-5554          device product:sdk_gphone64 model:sdk_gphone64_x86_64 device:emu64xa transport_id:1
adb-5HR8PVAIQ8AYPRKJ-YCcfVj._adb-tls-connect._tcp device product:rothko_eea model:2407FPN8EG device:rothko transport_id:3
192.168.1.5:5555       unauthorized transport_id:4
ABC123                 device usb:1-1 product:x model:Pixel_7 device:y transport_id:5
`
	d := ParseDevices(out)
	if len(d) != 4 {
		t.Fatalf("got %d devices", len(d))
	}
	want := []struct{ serial, transport, state, model string }{
		{"emulator-5554", "emulator", "device", "sdk gphone64 x86 64"},
		{"adb-5HR8PVAIQ8AYPRKJ-YCcfVj._adb-tls-connect._tcp", "wifi", "device", "2407FPN8EG"},
		{"192.168.1.5:5555", "wifi", "unauthorized", ""},
		{"ABC123", "usb", "device", "Pixel 7"},
	}
	for i, w := range want {
		if d[i].Serial != w.serial || d[i].Transport != w.transport || d[i].State != w.state || d[i].Model != w.model {
			t.Errorf("%d: %+v", i, d[i])
		}
	}
}

func TestReadMetaAndPackets(t *testing.T) {
	var b bytes.Buffer
	name := make([]byte, 64)
	copy(name, "Pixel 7")
	b.Write(name)
	b.WriteString("h264")
	binary.Write(&b, binary.BigEndian, uint32(1080))
	binary.Write(&b, binary.BigEndian, uint32(2400))
	pkt := func(flags uint64, pts uint64, data string) {
		binary.Write(&b, binary.BigEndian, flags<<62|pts)
		binary.Write(&b, binary.BigEndian, uint32(len(data)))
		b.WriteString(data)
	}
	pkt(2, 0, "cfg") // config bit (63)
	pkt(1, 42, "key")
	pkt(0, 43, "delta")
	m, err := ReadMeta(&b)
	if err != nil || m.Name != "Pixel 7" || m.Codec != "h264" || m.Width != 1080 || m.Height != 2400 {
		t.Fatalf("meta %+v %v", m, err)
	}
	p, _ := ReadPacket(&b)
	if !p.Config || p.Key || string(p.Data) != "cfg" {
		t.Errorf("config %+v", p)
	}
	p, _ = ReadPacket(&b)
	if p.Config || !p.Key || p.PTS != 42 || string(p.Data) != "key" {
		t.Errorf("key %+v", p)
	}
	p, _ = ReadPacket(&b)
	if p.Config || p.Key || p.PTS != 43 || string(p.Data) != "delta" {
		t.Errorf("delta %+v", p)
	}
	if _, err := ReadPacket(&b); err == nil {
		t.Error("expected EOF")
	}
}

func TestInputValidation(t *testing.T) {
	for _, ok := range []string{"192.168.1.5:37000", "phone.local:5555"} {
		if !hostPort.MatchString(ok) {
			t.Errorf("%s should pass", ok)
		}
	}
	for _, bad := range []string{"-s:1", "a b:1", "1.2.3.4", "x:99999999", ";rm:1"} {
		if hostPort.MatchString(bad) {
			t.Errorf("%s should fail", bad)
		}
	}
	if !avdName.MatchString("Xiaomi_14T_Pro") || avdName.MatchString("-avd") || avdName.MatchString("a b") {
		t.Error("avd names")
	}
	if !pairCode.MatchString("123456") || pairCode.MatchString("12 34") {
		t.Error("pair code")
	}
}
