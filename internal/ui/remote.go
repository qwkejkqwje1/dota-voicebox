package ui

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
)

// Пульт с телефона: отдельный маленький веб-сервер на всех адресах компьютера (сам интерфейс
// VoiceBox слушает только 127.0.0.1). Телефон в той же сети Wi-Fi открывает адрес из QR-кода —
// в нём секретный ключ — и видит плитки звуков, эффекты и кнопку микрофона.
// Пускаем только устройства из локальной сети и только с верным ключом.

// Remote — сервер пульта.
type Remote struct {
	App *app.App

	mu      sync.Mutex
	opts    config.Remote
	port    int
	srv     *http.Server
	errText string
	devices map[string]*Device
	fails   map[netip.Addr]*failCount
}

// Device — телефон (или планшет), который открывал пульт.
type Device struct {
	IP     string    `json:"ip"`
	Name   string    `json:"name"`
	First  time.Time `json:"first"`
	Seen   time.Time `json:"seen"`
	Online bool      `json:"online"`
	conns  int
}

type failCount struct {
	n     int
	since time.Time
	until time.Time
}

// RemoteStatus — состояние пульта для страницы «Телефон».
type RemoteStatus struct {
	Settings config.Remote `json:"settings"`
	Running  bool          `json:"running"`
	Port     int           `json:"port"`
	Error    string        `json:"error,omitempty"`
	URLs     []RemoteURL   `json:"urls"`
	Local    string        `json:"local,omitempty"` // адрес для предпросмотра на этом компьютере
	Devices  []Device      `json:"devices"`
}

// RemoteURL — адрес пульта в одной из сетей компьютера.
type RemoteURL struct {
	URL     string `json:"url"`
	IP      string `json:"ip"`
	Iface   string `json:"iface"`
	Primary bool   `json:"primary"`
	VPN     bool   `json:"vpn,omitempty"`
	QR      string `json:"qr,omitempty"` // SVG
}

// Apply включает, выключает или перезапускает сервер по настройкам (можно звать сколько угодно раз).
func (r *Remote) Apply(c *config.Config) {
	if r == nil || c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	oldKey := r.opts.Key
	r.opts = c.Remote
	if !c.Remote.Enabled || c.Remote.Key == "" {
		r.stopLocked()
		r.errText = ""
		return
	}
	port := c.RemotePort()
	if r.srv != nil && r.port == port && oldKey == c.Remote.Key {
		return
	}
	// новый ключ — перезапуск: телефоны со старой ссылкой отключаются сразу
	r.stopLocked()
	r.port = port
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		r.errText = fmt.Sprintf("Порт %d занят другой программой — выберите другой порт пульта", port)
		log.Printf("Пульт с телефона: %v", err)
		return
	}
	r.errText = ""
	srv := &http.Server{Handler: http.HandlerFunc(r.serve), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	r.srv = srv
	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("Пульт с телефона: %v", err)
		}
	}()
	log.Printf("Пульт с телефона: порт %d", port)
}

func (r *Remote) stopLocked() {
	if r.srv != nil {
		r.srv.Close()
		r.srv = nil
		log.Print("Пульт с телефона выключен")
	}
	for _, d := range r.devices {
		d.conns, d.Online = 0, false
	}
}

// Close останавливает сервер (выход из программы).
func (r *Remote) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.stopLocked()
	r.mu.Unlock()
}

// ForgetDevices — забыть телефоны (после смены ключа).
func (r *Remote) ForgetDevices() {
	r.mu.Lock()
	r.devices = nil
	r.mu.Unlock()
}

// Status — состояние, адреса с QR-кодами и телефоны.
func (r *Remote) Status() RemoteStatus {
	r.mu.Lock()
	st := RemoteStatus{Settings: r.opts, Running: r.srv != nil, Port: r.port, Error: r.errText, Devices: []Device{}, URLs: []RemoteURL{}}
	for _, d := range r.devices {
		st.Devices = append(st.Devices, *d)
	}
	r.mu.Unlock()
	sort.Slice(st.Devices, func(i, j int) bool { return st.Devices[i].Seen.After(st.Devices[j].Seen) })
	if !st.Running {
		return st
	}
	key := st.Settings.Key
	for _, a := range LANAddrs() {
		u := fmt.Sprintf("http://%s:%d/%s/", a.IP, st.Port, key)
		st.URLs = append(st.URLs, RemoteURL{URL: u, IP: a.IP, Iface: a.Iface, Primary: a.Primary, VPN: a.VPN, QR: qrSVG(u)})
	}
	st.Local = fmt.Sprintf("http://127.0.0.1:%d/%s/", st.Port, key)
	return st
}

// ---------- запросы с телефона ----------

// allowedClient — только локальная сеть: частные адреса, link-local и сам компьютер.
func allowedClient(ip netip.Addr) bool {
	return ip.IsValid() && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func clientIP(req *http.Request) netip.Addr {
	ap, err := netip.ParseAddrPort(req.RemoteAddr)
	if err != nil {
		return netip.Addr{}
	}
	return ap.Addr().Unmap()
}

const (
	failLimit  = 50               // неверных ключей с одного адреса…
	failWindow = 10 * time.Minute // …за это время — и адрес блокируется на столько же
)

func (r *Remote) blocked(ip netip.Addr, now time.Time) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	f := r.fails[ip]
	return f != nil && now.Before(f.until)
}

func (r *Remote) fail(ip netip.Addr, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fails == nil {
		r.fails = map[netip.Addr]*failCount{}
	}
	f := r.fails[ip]
	if f == nil || now.Sub(f.since) > failWindow {
		f = &failCount{since: now}
		r.fails[ip] = f
	}
	f.n++
	if f.n >= failLimit {
		f.until = now.Add(failWindow)
		log.Printf("Пульт с телефона: %s %d раз ошибся адресом — не пускаем %v", ip, f.n, failWindow)
	}
	if len(r.fails) > 1000 { // не копить мусор
		for k, v := range r.fails {
			if now.Sub(v.since) > failWindow && now.After(v.until) {
				delete(r.fails, k)
			}
		}
	}
}

func (r *Remote) keyOK(k string) bool {
	r.mu.Lock()
	key := r.opts.Key
	r.mu.Unlock()
	return key != "" && subtle.ConstantTimeCompare([]byte(k), []byte(key)) == 1
}

// touch запоминает телефон; conn — открыл (+1) или закрыл (-1) поток событий.
func (r *Remote) touch(ip netip.Addr, ua string, conn int) {
	if ip.IsLoopback() { // предпросмотр в окне VoiceBox — не телефон
		return
	}
	now := time.Now()
	r.mu.Lock()
	if r.devices == nil {
		r.devices = map[string]*Device{}
	}
	k := ip.String()
	d := r.devices[k]
	fresh := d == nil
	if fresh {
		d = &Device{IP: k, First: now}
		r.devices[k] = d
	}
	if ua != "" {
		d.Name = deviceName(ua)
	}
	d.Seen = now
	d.conns = max(d.conns+conn, 0)
	d.Online = d.conns > 0
	name := d.Name
	r.mu.Unlock()
	if fresh {
		log.Printf("Пульт с телефона: подключилось устройство %s (%s)", name, k)
		r.App.Notify(app.Toast{Emoji: "📱", Text: "Телефон подключён", Sub: name})
	}
}

// deviceName — понятное имя устройства по User-Agent.
func deviceName(ua string) string {
	switch {
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Android"):
		if _, rest, ok := strings.Cut(ua, "Android"); ok {
			if i := strings.IndexByte(rest, ')'); i > 0 {
				parts := strings.Split(rest[:i], ";")
				if len(parts) >= 2 {
					m := strings.TrimSpace(parts[len(parts)-1])
					m, _, _ = strings.Cut(m, " Build/")
					if len(m) > 1 && len(m) < 40 && !strings.EqualFold(m, "wv") {
						return "Android · " + m
					}
				}
			}
		}
		return "Android"
	case strings.Contains(ua, "Windows"):
		return "Компьютер с Windows"
	case strings.Contains(ua, "Macintosh"):
		return "Mac или iPad"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	}
	return "Устройство"
}

const remoteNotFound = `<!doctype html><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>VoiceBox</title><body style="font:16px system-ui;background:#0b0b10;color:#e4e4e7;padding:32px;line-height:1.5">
<h2 style="margin:0 0 8px">📱 Пульт VoiceBox</h2><p style="color:#a1a1aa">Адрес неполный или устарел. Откройте в VoiceBox
страницу «Телефон» и отсканируйте QR-код ещё раз.</p></body>`

func (r *Remote) serve(w http.ResponseWriter, req *http.Request) {
	ip := clientIP(req)
	if !allowedClient(ip) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer") // ключ из адреса не уходит на сторонние сайты
	h.Set("Cache-Control", "no-store")
	now := time.Now()
	if r.blocked(ip, now) {
		http.Error(w, i18n.T("слишком много неверных попыток, подождите"), http.StatusTooManyRequests)
		return
	}
	key, rest, slash := strings.Cut(strings.TrimPrefix(req.URL.Path, "/"), "/")
	if !r.keyOK(key) {
		if len(key) >= 6 && !strings.Contains(key, ".") { // favicon.ico и прочие запросы браузера — не попытки
			r.fail(ip, now)
		}
		h.Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, i18n.HTML(remoteNotFound))
		return
	}
	if !slash { // /ключ → /ключ/ (иначе относительные адреса страницы не сработают)
		http.Redirect(w, req, "/"+key+"/", http.StatusFound)
		return
	}
	switch {
	case rest == "api/events" && req.Method == http.MethodGet:
		r.events(w, req, ip)
		return
	case strings.HasPrefix(rest, "api/"):
		r.touch(ip, req.UserAgent(), 0)
		r.api(w, req, strings.TrimPrefix(rest, "api/"))
		return
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	file := ""
	switch rest {
	case "", "index.html":
		file = "remote/index.html"
		r.touch(ip, req.UserAgent(), 0)
	case "remote.js", "remote.css", "manifest.json":
		file = "remote/" + rest
	case "icon.png":
		file = "icon.png"
	default:
		http.NotFound(w, req)
		return
	}
	b, err := fs.ReadFile(uiFiles(), file)
	if err != nil {
		http.NotFound(w, req)
		return
	}
	if t := staticTypes[path.Ext(file)]; t != "" {
		h.Set("Content-Type", t)
	}
	switch file { // английский интерфейс — и пульт по-английски
	case "remote/index.html":
		b = []byte(i18n.HTML(string(b)))
	case "remote/remote.js":
		b = i18n.JS(b)
	case "remote/manifest.json":
		h.Set("Content-Type", "application/manifest+json")
		b = i18n.TranslateJSON(b)
	}
	w.Write(b)
}

// uiFiles — встроенные файлы интерфейса (или с диска при разработке).
func uiFiles() fs.FS {
	if dir := os.Getenv("VOICEBOX_UI_DIR"); dir != "" {
		return os.DirFS(dir)
	}
	sub, _ := fs.Sub(static, "static")
	return sub
}

// remoteState — всё, что нужно странице пульта.
type remoteState struct {
	Rev     int64          `json:"rev"`
	Version string         `json:"version"`
	Sounds  []app.MenuItem `json:"sounds"`
	Effects []app.MenuItem `json:"effects"`
	Preset  app.MenuItem   `json:"preset"`
	ShowMic bool           `json:"show_mic"`
	Live    remoteLive     `json:"live"`
	Sets    []app.MenuItem `json:"sets,omitempty"` // наборы звуков (первый — «Все звуки»); Active — включённый
}

// remoteLive — то, что меняется часто: что играет, эффект, микрофон.
type remoteLive struct {
	Rev     int64        `json:"rev"`
	Playing []remotePlay `json:"playing"`
	Preset  string       `json:"preset"`
	Mute    bool         `json:"mute"`
	Talk    string       `json:"talk,omitempty"`
	Busy    bool         `json:"busy,omitempty"` // готовится речь
}

type remotePlay struct {
	ID  string  `json:"id"`
	Pos float64 `json:"pos"`
	Len float64 `json:"len"`
}

func (r *Remote) live() remoteLive {
	st := r.App.Status()
	l := remoteLive{Rev: st.Rev, Preset: st.Preset, Mute: st.MicMute, Busy: st.TTSBusy, Playing: []remotePlay{}}
	seen := map[string]bool{}
	for _, p := range r.App.Playing() {
		if !seen[p.ID] {
			seen[p.ID] = true
			l.Playing = append(l.Playing, remotePlay{ID: p.ID, Pos: float64(int(p.Pos*10)) / 10, Len: float64(int(p.Len*10)) / 10})
		}
	}
	sort.Slice(l.Playing, func(i, j int) bool { return l.Playing[i].ID < l.Playing[j].ID })
	return l
}

func (r *Remote) state() remoteState {
	c := r.App.Config()
	st := remoteState{Version: r.App.Version, ShowMic: c.Remote.ShowMic, Effects: []app.MenuItem{}, Live: r.live()}
	st.Rev = st.Live.Rev
	if c.Remote.Sounds == "menu" {
		st.Sounds = r.App.Menu().Sounds
	} else {
		st.Sounds = r.App.PadItems()
	}
	if st.Sounds == nil {
		st.Sounds = []app.MenuItem{}
	}
	if c.Remote.ShowEffects {
		st.Effects = r.App.EffectItems()
	}
	title, emoji := app.PresetTitle(st.Live.Preset)
	st.Preset = app.MenuItem{ID: st.Live.Preset, Label: title, Emoji: emoji}
	if len(c.Sets) > 0 && c.Remote.Sounds != "menu" {
		ids := []string{""}
		for _, s := range c.Sets {
			ids = append(ids, s.ID)
		}
		for _, id := range ids {
			title, emoji := app.SetTitle(c, id)
			st.Sets = append(st.Sets, app.MenuItem{ID: id, Label: title, Emoji: emoji, Active: id == c.ActiveSet})
		}
	}
	return st
}

func (r *Remote) api(w http.ResponseWriter, req *http.Request, name string) {
	var res any
	var err error
	post := req.Method == http.MethodPost
	var body struct {
		ID string `json:"id"`
	}
	if post {
		json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096)).Decode(&body)
	}
	c := r.App.Config()
	switch {
	case name == "state" && req.Method == http.MethodGet:
		res = r.state()
	case name == "play" && post:
		err = r.App.PlayOpts(body.ID, 1)
	case name == "stop" && post:
		r.App.StopAll()
	case name == "preset" && post && c.Remote.ShowEffects:
		r.App.SetPreset(body.ID, true)
	case name == "mic" && post && c.Remote.ShowMic:
		r.App.Do("mic_mute")
	case name == "set" && post:
		err = r.App.SetActiveSet(body.ID, true)
	default:
		http.NotFound(w, req)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		msg := i18n.T(err.Error())
		msg = strings.TrimPrefix(strings.TrimPrefix(msg, "звук "), "sound ")
		json.NewEncoder(w).Encode(map[string]string{"error": msg})
		return
	}
	if res == nil {
		res = r.live()
	}
	b, _ := i18n.JSON(res)
	w.Write(append(b, '\n'))
}

// events — поток событий для телефона: что играет, эффект, микрофон (при изменениях) и «пинг».
func (r *Remote) events(w http.ResponseWriter, req *http.Request, ip netip.Addr) {
	fl, ok := w.(http.Flusher)
	if !ok {
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no")
	r.touch(ip, req.UserAgent(), +1)
	defer r.touch(ip, "", -1)
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	last, lastSent := "", time.Now()
	fmt.Fprint(w, "retry: 2000\n\n")
	for {
		b, _ := i18n.JSON(r.live())
		switch {
		case string(b) != last:
			fmt.Fprintf(w, "event: live\ndata: %s\n\n", b)
			last, lastSent = string(b), time.Now()
			fl.Flush()
		case time.Since(lastSent) > 15*time.Second: // соединение живо (и телефон видит, что связь есть)
			fmt.Fprint(w, ": ping\n\n")
			lastSent = time.Now()
			fl.Flush()
		}
		select {
		case <-req.Context().Done():
			return
		case <-tick.C:
		}
	}
}
