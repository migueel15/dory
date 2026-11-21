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

func TestParseForeground(t *testing.T) {
	dump := "  mResumedActivity: ActivityRecord{1a2b3c u0 com.android.settings/.Settings$Sub t12}\n"
	p, a := ParseForeground(dump)
	if p != "com.android.settings" || a != ".Settings$Sub" {
		t.Errorf("%q %q", p, a)
	}
}

func TestParseUI(t *testing.T) {
	dump := `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?><hierarchy rotation="0"><node index="0" text="" resource-id="" class="android.widget.FrameLayout" clickable="false" bounds="[0,0][1080,2400]"><node index="0" text="Buscar" resource-id="com.whatsapp:id/search" class="android.widget.EditText" clickable="true" bounds="[100,200][500,300]" /><node text="" resource-id="x:id/deco" class="android.view.View" clickable="false" bounds="[0,0][10,10]" /><node text="" content-desc="Más opciones" class="android.widget.ImageView" clickable="true" bounds="[900,100][1000,200]" /></node></hierarchy>
UI hierchary dumped to: /dev/tty`
	els, err := ParseUI(dump)
	if err != nil || len(els) != 2 {
		t.Fatalf("%v %+v", err, els)
	}
	if els[0].Text != "Buscar" || els[0].ID != "search" || els[0].CenterX != 300 || els[0].CenterY != 250 || els[1].Desc != "Más opciones" || els[1].Index != 1 {
		t.Errorf("%+v", els)
	}
	m := NewManager()
	m.lastUI = map[string][]UIElement{"s": els}
	x, y, _, err := m.Resolve("s", Target{Text: "más opciones"})
	if err != nil || x != 950 || y != 150 {
		t.Errorf("%d %d %v", x, y, err)
	}
}

func TestLauncherAndFindApp(t *testing.T) {
	out := `3 activities found:
  Activity #0:
    priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true
    com.whatsapp/.Main
  Activity #1:
    priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true
    com.whatsapp.w4b/.Main
  Activity #2:
    priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true
    com.android.settings/.Settings
`
	apps := ParseLauncher(out)
	if len(apps) != 3 {
		t.Fatalf("%+v", apps)
	}
	if got := FindApp(apps, "WhatsApp"); len(got) != 1 || got[0].Package != "com.whatsapp" {
		t.Errorf("whatsapp: %+v", got)
	}
	if got := FindApp(apps, "ajustes"); len(got) != 1 || got[0].Package != "com.android.settings" {
		t.Errorf("ajustes: %+v", got)
	}
	if got := FindApp(apps, "w4b"); len(got) != 1 {
		t.Errorf("w4b: %+v", got)
	}
	if got := FindApp(apps, "nada"); len(got) != 0 {
		t.Errorf("nada: %+v", got)
	}
}

func TestParseNotifications(t *testing.T) {
	dump := `    NotificationRecord(0x05b988e8: pkg=net.dorlab.dory user=UserHandle{0} id=1 tag=null importance=4 key=0|x bbbc=0: Notification(channel=n))
      icon=Icon(typ=RESOURCE pkg=net.dorlab.dory id=0x7f0c0000)
                android.title=String (Un agente te necesita)
                android.text=String (Abrir el panel)
    NotificationRecord(0x0d0453e7: pkg=android user=UserHandle{-1} id=0 tag=-1|android importance=4 key=k bbbc=0: Notification(channel=D))
      icon=Icon(typ=RESOURCE pkg=android id=0x1)
`
	n := ParseNotifications(dump)
	if len(n) != 1 || n[0].Package != "net.dorlab.dory" || n[0].Title != "Un agente te necesita" || n[0].Text != "Abrir el panel" {
		t.Errorf("%+v", n)
	}
}

func TestResolveContains(t *testing.T) {
	m := NewManager()
	m.lastUI = map[string][]UIElement{"s": {{Index: 0, Text: "Buscar o escribir", CenterX: 5, CenterY: 6}, {Index: 1, Text: "Buscar", CenterX: 1, CenterY: 2}}}
	if x, _, _, err := m.Resolve("s", Target{Text: "buscar"}); err != nil || x != 1 {
		t.Errorf("exact should win: %d %v", x, err)
	}
	if x, _, _, err := m.Resolve("s", Target{Text: "escribir"}); err != nil || x != 5 {
		t.Errorf("contains: %d %v", x, err)
	}
}
