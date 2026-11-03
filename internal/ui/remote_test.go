package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
)

type nopOut struct{ v, m audio.Mixer }

func (f *nopOut) Voice() *audio.Mixer                             { return &f.v }
func (f *nopOut) Monitor() *audio.Mixer                           { return &f.m }
func (f *nopOut) SetChain(*dsp.Chain)                             {}
func (f *nopOut) ToggleMicMonitor() bool                          { return true }
func (f *nopOut) MicMonitor() bool                                { return false }
func (f *nopOut) Running() bool                                   { return true }
func (f *nopOut) Start(audio.Options) error                       { return nil }
func (f *nopOut) SetMix(float64, float64, float64, float64, bool) {}
func (f *nopOut) Status() audio.Status                            { return audio.Status{Running: true} }

const testKey = "abcdefgh2345"

func newRemote(t *testing.T, edit func(c *config.Config)) (*Remote, *app.App) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(p, config.DefaultJSON, 0o644)
	a := app.New(p, &nopOut{})
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	err := a.UpdateConfig(func(c *config.Config) {
		c.Remote.Enabled, c.Remote.Key = true, testKey
		if edit != nil {
			edit(c)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	r := &Remote{App: a}
	r.opts = a.Config().Remote // без сервера: запросы идут прямо в serve
	return r, a
}

func call(r *Remote, ip, method, path, body string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = ip + ":51000"
	if strings.Contains(ip, ":") {
		req.RemoteAddr = "[" + ip + "]:51000"
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	r.serve(w, req)
	return w
}

func TestRemoteAccess(t *testing.T) {
	r, a := newRemote(t, nil)
	const phone = "192.168.1.23"
	if w := call(r, "8.8.8.8", "GET", "/"+testKey+"/", ""); w.Code != http.StatusForbidden {
		t.Fatalf("адрес не из локальной сети: %d", w.Code)
	}
	if w := call(r, phone, "GET", "/"+testKey, ""); w.Code != http.StatusFound || w.Header().Get("Location") != "/"+testKey+"/" {
		t.Fatalf("редирект на адрес со слешем: %d %q", w.Code, w.Header().Get("Location"))
	}
	ua := "Mozilla/5.0 (Linux; Android 14; Pixel 8 Build/AP2A) AppleWebKit/537.36 Chrome/126 Mobile"
	w := call(r, phone, "GET", "/"+testKey+"/", "", "User-Agent", ua)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "remote.js") || w.Header().Get("Referrer-Policy") != "no-referrer" ||
		w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("страница пульта: %d %v", w.Code, w.Header())
	}
	for _, f := range []string{"remote.js", "remote.css", "manifest.json", "icon.png"} {
		if w := call(r, phone, "GET", "/"+testKey+"/"+f, ""); w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("%s: %d", f, w.Code)
		}
	}
	if w := call(r, phone, "GET", "/"+testKey+"/../config.json", ""); w.Code == 200 {
		t.Fatal("лишние файлы отдавать нельзя")
	}
	if w := call(r, phone, "GET", "/"+testKey+"/app.js", ""); w.Code != http.StatusNotFound {
		t.Fatalf("файлы интерфейса компьютера пульту не нужны: %d", w.Code)
	}
	if w := call(r, phone, "POST", "/"+testKey+"/", ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatal(w.Code)
	}
	st := r.Status()
	if len(st.Devices) != 1 || st.Devices[0].IP != phone || st.Devices[0].Name != "Android · Pixel 8" || st.Running {
		t.Fatalf("%+v", st)
	}
	call(r, "127.0.0.1", "GET", "/"+testKey+"/", "")
	if len(r.Status().Devices) != 1 {
		t.Fatal("предпросмотр на этом компьютере — не телефон")
	}
	r.ForgetDevices()
	if len(r.Status().Devices) != 0 {
		t.Fatal("ForgetDevices")
	}
	_ = a
}

func TestRemoteWrongKeyLimit(t *testing.T) {
	r, _ := newRemote(t, nil)
	const guesser, other = "10.0.0.7", "10.0.0.8"
	for i := 0; i < 50; i++ { // запросы браузера без ключа не считаются попытками
		call(r, guesser, "GET", "/favicon.ico", "")
		call(r, guesser, "GET", "/", "")
	}
	if w := call(r, guesser, "GET", "/"+testKey+"/", ""); w.Code != 200 {
		t.Fatalf("favicon не попытка подбора: %d", w.Code)
	}
	for i := 0; i < failLimit; i++ {
		w := call(r, guesser, "GET", "/zzzzzzzz"+strings.Repeat("a", i%3)+"/", "")
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "Телефон") {
			t.Fatalf("неверный ключ: %d", w.Code)
		}
	}
	if w := call(r, guesser, "GET", "/"+testKey+"/", ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("после %d ошибок адрес блокируется: %d", failLimit, w.Code)
	}
	if w := call(r, other, "GET", "/"+testKey+"/", ""); w.Code != 200 {
		t.Fatal("блокировка только для одного адреса")
	}
	ip := netip.MustParseAddr(guesser)
	if !r.blocked(ip, time.Now().Add(failWindow-time.Second)) || r.blocked(ip, time.Now().Add(failWindow+time.Second)) {
		t.Fatal("блокировка на failWindow")
	}
}

func TestRemoteAPI(t *testing.T) {
	r, a := newRemote(t, func(c *config.Config) { c.Remote.ShowEffects = false })
	const phone = "192.168.0.50"
	w := call(r, phone, "GET", "/"+testKey+"/api/state", "")
	var st remoteState
	if err := json.Unmarshal(w.Body.Bytes(), &st); err != nil || w.Code != 200 {
		t.Fatalf("%d %v %s", w.Code, err, w.Body)
	}
	if len(st.Sounds) < 8 || st.Sounds[0].ID != "airhorn" || st.Sounds[0].Label == "" || len(st.Effects) != 0 || !st.ShowMic || st.Live.Playing == nil {
		t.Fatalf("%+v", st)
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/play", `{"id":"airhorn"}`); w.Code != 200 || !a.IsPlaying("airhorn") ||
		!strings.Contains(w.Body.String(), `"airhorn"`) {
		t.Fatalf("play: %d %s", w.Code, w.Body)
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/play", `{"id":"nope"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "error") {
		t.Fatalf("неизвестный звук: %d %s", w.Code, w.Body)
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/stop", ``); w.Code != 200 || a.IsPlaying("airhorn") {
		t.Fatal("stop")
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/preset", `{"id":"dead_inside"}`); w.Code != http.StatusNotFound {
		t.Fatal("эффекты выключены в настройках пульта")
	}
	if w := call(r, phone, "GET", "/"+testKey+"/api/play", ``); w.Code != http.StatusNotFound {
		t.Fatal("play только POST")
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/mic", ``); w.Code != 200 || !a.Config().Mic.Mute {
		t.Fatal("mic")
	}
	a.UpdateConfig(func(c *config.Config) { c.Remote.ShowEffects, c.Remote.ShowMic, c.Remote.Sounds = true, false, "menu" })
	if w := call(r, phone, "POST", "/"+testKey+"/api/mic", ``); w.Code != http.StatusNotFound {
		t.Fatal("кнопка микрофона спрятана")
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/preset", `{"id":"dead_inside"}`); w.Code != 200 || !strings.Contains(w.Body.String(), `"preset":"dead_inside"`) {
		t.Fatalf("preset: %s", w.Body)
	}
	json.Unmarshal(call(r, phone, "GET", "/"+testKey+"/api/state", "").Body.Bytes(), &st)
	if len(st.Effects) == 0 || st.Preset.ID != "dead_inside" || st.Preset.Label == "" || st.ShowMic || len(st.Sets) != 0 {
		t.Fatalf("%+v", st)
	}

	// наборы звуков: переключаются с телефона, плитки — из набора
	a.UpdateConfig(func(c *config.Config) {
		c.Remote.Sounds = "pad"
		c.Sets = []config.SoundSet{{ID: "set_cs", Name: "CS2", Emoji: "🔫", Sounds: []string{"boing", "airhorn"}}}
	})
	st = remoteState{}
	json.Unmarshal(call(r, phone, "GET", "/"+testKey+"/api/state", "").Body.Bytes(), &st)
	if len(st.Sets) != 2 || st.Sets[0].ID != "" || !st.Sets[0].Active || st.Sets[1].Label != "CS2" || st.Sets[1].Active {
		t.Fatalf("наборы: %+v", st.Sets)
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/set", `{"id":"set_cs"}`); w.Code != 200 || a.Config().ActiveSet != "set_cs" {
		t.Fatalf("set: %d %s", w.Code, w.Body)
	}
	st = remoteState{}
	json.Unmarshal(call(r, phone, "GET", "/"+testKey+"/api/state", "").Body.Bytes(), &st)
	if len(st.Sounds) != 2 || st.Sounds[0].ID != "boing" || !st.Sets[1].Active {
		t.Fatalf("плитки набора: %+v", st.Sounds)
	}
	if w := call(r, phone, "POST", "/"+testKey+"/api/set", `{"id":"nope"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("неизвестный набор: %d", w.Code)
	}
}

func TestRemoteServerAndEvents(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	r, a := newRemote(t, func(c *config.Config) { c.Remote.Port = port })
	defer r.Close()
	r.Apply(a.Config())
	st := r.Status()
	if !st.Running || st.Port != port || st.Error != "" || !strings.HasSuffix(st.Local, "/"+testKey+"/") {
		t.Fatalf("%+v", st)
	}
	for _, u := range st.URLs {
		if !strings.Contains(u.URL, testKey) || !strings.HasPrefix(u.QR, "<svg") {
			t.Fatalf("%+v", u)
		}
	}
	get := func(u string) int {
		res, err := http.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if c := get(st.Local); c != 200 {
		t.Fatal(c)
	}
	r.Apply(a.Config()) // те же настройки — сервер не перезапускается
	if c := get(st.Local + "api/state"); c != 200 {
		t.Fatal(c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", st.Local+"api/events", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(res.Body)
	var lines []string
	for sc.Scan() && len(lines) < 3 {
		if l := sc.Text(); l != "" {
			lines = append(lines, l)
		}
	}
	res.Body.Close()
	if len(lines) < 3 || lines[0] != "retry: 2000" || lines[1] != "event: live" || !strings.HasPrefix(lines[2], `data: {"rev":`) {
		t.Fatalf("%q", lines)
	}
	if err := a.SaveRemoteKey("zyxwvuts9876"); err != nil {
		t.Fatal(err)
	}
	r.Apply(a.Config())
	if c := get(st.Local); c != http.StatusNotFound {
		t.Fatalf("старая ссылка после смены ключа: %d", c)
	}
	if c := get(strings.Replace(st.Local, testKey, "zyxwvuts9876", 1)); c != 200 {
		t.Fatal(c)
	}
	busy, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	a.UpdateConfig(func(c *config.Config) { c.Remote.Port = busy.Addr().(*net.TCPAddr).Port })
	r.Apply(a.Config())
	if st := r.Status(); st.Running || !strings.Contains(st.Error, "занят") {
		t.Fatalf("занятый порт: %+v", st)
	}
	a.UpdateConfig(func(c *config.Config) { c.Remote.Enabled = false })
	r.Apply(a.Config())
	if st := r.Status(); st.Running || st.Error != "" {
		t.Fatalf("%+v", st)
	}
}

func TestRemoteHelpers(t *testing.T) {
	for ip, ok := range map[string]bool{"192.168.1.2": true, "10.1.2.3": true, "172.20.0.1": true, "127.0.0.1": true, "::1": true,
		"fe80::1": true, "fd12::1": true, "169.254.3.4": true, "8.8.8.8": false, "100.64.0.1": false, "2001:db8::1": false} {
		if allowedClient(netip.MustParseAddr(ip)) != ok {
			t.Fatalf("%s: %v", ip, !ok)
		}
	}
	if allowedClient(netip.Addr{}) {
		t.Fatal("пустой адрес")
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::ffff:192.168.1.5]:1234"
	if ip := clientIP(req); ip.String() != "192.168.1.5" {
		t.Fatal(ip)
	}
	for ua, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15":              "iPhone",
		"Mozilla/5.0 (Linux; Android 13; SM-A536B) AppleWebKit/537.36 Chrome/125 Mobile":           "Android · SM-A536B",
		"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 Chrome/125 Mobile":                  "Android",
		"Mozilla/5.0 (Linux; Android 12; wv) AppleWebKit/537.36":                                   "Android",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36":                             "Компьютер с Windows",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Version/17.5 Safari": "Mac или iPad",
		"curl/8.0": "Устройство",
	} {
		if got := deviceName(ua); got != want {
			t.Fatalf("%q → %q, ждали %q", ua, got, want)
		}
	}
}

func TestLANKinds(t *testing.T) {
	for name, want := range map[string][2]bool{
		"Wi-Fi": {false, false}, "Ethernet 2": {false, false}, "Беспроводная сеть": {false, false}, "wlan0": {false, false},
		"vEthernet (WSL)": {true, false}, "VirtualBox Host-Only Network": {true, false}, "docker0": {true, false},
		"NordLynx": {false, true}, "ProtonVPN": {false, true}, "Radmin VPN": {false, true}, "ZeroTier One [8056c2e21c]": {false, true},
		"OpenVPN TAP-Windows6": {false, true}, "tun0": {false, true}, "wg0": {false, true}, "AmneziaVPN": {false, true},
	} {
		v, p := ifaceKind(name)
		if v != want[0] || p != want[1] {
			t.Fatalf("%q: virtual=%v vpn=%v", name, v, p)
		}
	}
	got := sortLAN([]LANAddr{{IP: "10.8.0.2", VPN: true}, {IP: "192.168.56.1"}, {IP: "192.168.1.5", Primary: true}, {IP: "10.0.0.3"}})
	var ips []string
	for _, a := range got {
		ips = append(ips, a.IP)
	}
	if strings.Join(ips, " ") != "192.168.1.5 192.168.56.1 10.0.0.3 10.8.0.2" {
		t.Fatal(ips)
	}
}
