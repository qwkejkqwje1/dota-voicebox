// Package ui — локальный веб-интерфейс (открывается в окне WebView2 или в браузере).
package ui

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/logbus"
	"github.com/qwkejkqwje1/dota-voicebox/internal/script"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/update"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

//go:embed static
var static embed.FS

type Server struct {
	App    *app.App
	Engine *audio.Engine
	Logs   *logbus.Bus
	Token  string
	Addr   string
	OnShow func()
	OnQuit func()

	Updater     *update.Updater
	RestartArgs []string
}

func NewToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// URL — адрес интерфейса с токеном доступа.
func (s *Server) URL() string { return "http://" + s.Addr + "/?t=" + s.Token }

// Listen занимает порт. Ошибка — значит программа уже запущена (или порт занят).
func (s *Server) Listen() (net.Listener, error) { return net.Listen("tcp", s.Addr) }

func (s *Server) Serve(l net.Listener) error {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(static, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "voicebox") })
	mux.HandleFunc("/api/show", func(w http.ResponseWriter, r *http.Request) {
		if s.OnShow != nil {
			go s.OnShow()
		}
	})
	api := map[string]func(*http.Request) (any, error){
		"GET /api/state":            s.state,
		"POST /api/action":          s.action,
		"PUT /api/config":           s.putConfig,
		"POST /api/sounds":          s.upload,
		"POST /api/sounds/delete":   s.deleteSound,
		"GET /api/setup":            func(*http.Request) (any, error) { return s.App.SetupStatus(), nil },
		"POST /api/setup/auto":      s.autoSetup,
		"POST /api/setup/fix":       s.fix,
		"POST /api/setup/autostart": s.autostart,
		"POST /api/hotkeys/pause":   s.pauseHotkeys,
		"POST /api/open":            s.open,
		"POST /api/quit":            s.quit,
		"POST /api/preset/test":     s.testPreset,
		"GET /api/scripts":          s.scriptsList,
		"GET /api/scripts/code":     s.scriptCode,
		"PUT /api/scripts":          s.scriptSave,
		"POST /api/scripts/delete":  s.scriptDelete,
		"POST /api/scripts/run":     s.scriptRun,
		"POST /api/scripts/enable":  s.scriptEnable,
		"POST /api/scripts/check":   s.scriptCheck,
		"POST /api/test":            s.runTest,
		"POST /api/clock":           s.setClock,
		"GET /api/update":           func(*http.Request) (any, error) { return s.Updater.Status(), nil },
		"POST /api/update/check":    s.updateCheck,
		"POST /api/update/install":  s.updateInstall,
		"POST /api/update/rollback": s.updateRollback,
		"GET /api/backups": func(*http.Request) (any, error) {
			return map[string]any{"backups": config.Backups(s.App.ConfigPath)}, nil
		},
		"POST /api/backups/restore": s.restoreBackup,
		"POST /api/diag/bundle":     s.diagBundle,
		"GET /api/apps/capture": func(*http.Request) (any, error) {
			l, err := winapi.CaptureSessions()
			return map[string]any{"sessions": l}, err
		},
	}
	for pattern, h := range api {
		h := h
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Token") != s.Token {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			res, err := h(r)
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			if res == nil {
				res = map[string]bool{"ok": true}
			}
			json.NewEncoder(w).Encode(res)
		})
	}
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/viz", s.viz)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	return srv.Serve(l)
}

func decode(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 8<<20)).Decode(v)
}

// ---------- состояние ----------

type presetInfo struct {
	Name    string     `json:"name"`
	Builtin bool       `json:"builtin"`
	Desc    string     `json:"desc"`
	Specs   []dsp.Spec `json:"specs"`
}

var presetDesc = map[string]string{
	"clean":           "Чистый голос",
	"dead_inside":     "Передавленный микрофон, перегруз и раздутый бас",
	"dead_inside_max": "Ушная боль: жёсткий клиппинг, 6 бит",
	"radio":           "Рация: узкая полоса, щелчок, шипение, хвост",
	"radio_broken":    "Рация с плохим приёмом: треск и выпадения",
	"megaphone":       "Громкоговоритель",
	"robot":           "Робот",
}

func (s *Server) state(*http.Request) (any, error) {
	cfg := s.App.Config()
	var presets []presetInfo
	for _, n := range s.App.PresetNames() {
		specs, builtin := dsp.BuiltinPresets[n]
		if c, ok := cfg.VoicePresets[n]; ok {
			specs, builtin = c, false
		}
		presets = append(presets, presetInfo{Name: n, Builtin: builtin, Desc: presetDesc[n], Specs: specs})
	}
	capture, playback, derr := audio.Devices()
	cable, cableRec := s.App.CableNames(capture, playback)
	errStr := ""
	if derr != nil {
		errStr = derr.Error()
	}
	return map[string]any{
		"version":    s.App.Version,
		"config":     cfg,
		"sounds":     s.App.Sounds(),
		"builtins":   sounds.BuiltinNames(),
		"presets":    presets,
		"devices":    map[string]any{"capture": capture, "playback": playback, "error": errStr, "cable": cable, "cable_rec": cableRec},
		"events":     gsiEvents,
		"logs":       s.Logs.Lines(),
		"status":     s.App.Status(),
		"sounds_dir": s.App.SoundsDir(),
	}, nil
}

var gsiEvents = []map[string]string{
	{"id": gsi.EvGameStart, "label": "Горн (0:00)"}, {"id": gsi.EvPreGame, "label": "Начало пре-гейма"},
	{"id": gsi.EvKill, "label": "Вы убили"}, {"id": gsi.EvDeath, "label": "Вы умерли"},
	{"id": gsi.EvRespawn, "label": "Возрождение"}, {"id": gsi.EvStreak3, "label": "Серия 3"},
	{"id": gsi.EvStreak5, "label": "Серия 5"}, {"id": gsi.EvStreak10, "label": "Серия 10"},
	{"id": gsi.EvLowHP, "label": "Мало HP"}, {"id": gsi.EvLevel6, "label": "6 уровень"},
	{"id": gsi.EvSmoked, "label": "Под смоком"}, {"id": gsi.EvDay, "label": "Наступил день"},
	{"id": gsi.EvNight, "label": "Наступила ночь"}, {"id": gsi.EvVictory, "label": "Победа"},
	{"id": gsi.EvDefeat, "label": "Поражение"}, {"id": "rosh_killed", "label": "Рошан убит (авто)"},
}

// events — поток Server-Sent Events: статус 10 раз в секунду + строки журнала.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("t") != s.Token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	logs := s.Logs.Subscribe()
	defer s.Logs.Unsubscribe(logs)
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	send := func(ev string, v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, b)
	}
	n := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case line := <-logs:
			send("log", line)
			fl.Flush()
		case <-tick.C:
			lv := s.Engine.Levels()
			send("levels", map[string]any{"mic_in": lv.MicIn, "voice_out": lv.VoiceOut, "monitor": lv.Monitor, "strips": s.Engine.StripLevels()})
			if n%5 == 0 { // статус — 2 раза в секунду
				send("status", s.App.Status())
			}
			n++
			fl.Flush()
		}
	}
}

// ---------- действия ----------

func (s *Server) action(r *http.Request) (any, error) {
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	s.App.Do(req.Action)
	return nil, nil
}

func (s *Server) putConfig(r *http.Request) (any, error) {
	var c config.Config
	if err := decode(r, &c); err != nil {
		return nil, err
	}
	if err := s.App.SaveConfig(&c); err != nil {
		return nil, err
	}
	return s.state(r)
}

var safeName = regexp.MustCompile(`[^\p{L}\p{N}_\-\. ]+`)

func (s *Server) upload(r *http.Request) (any, error) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, err
	}
	dir := s.App.SoundsDir()
	os.MkdirAll(dir, 0o755)
	var saved []string
	for _, fh := range r.MultipartForm.File["files"] {
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if ext != ".mp3" && ext != ".wav" {
			return nil, fmt.Errorf("%s: поддерживаются только .mp3 и .wav", fh.Filename)
		}
		name := strings.TrimSpace(safeName.ReplaceAllString(strings.TrimSuffix(filepath.Base(fh.Filename), filepath.Ext(fh.Filename)), "_"))
		name = strings.Trim(strings.ReplaceAll(name, " ", "_"), "_.")
		if name == "" {
			name = "sound"
		}
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		// проверяем, что файл декодируется, до сохранения
		tmp := filepath.Join(dir, ".upload"+ext)
		os.WriteFile(tmp, data, 0o644)
		_, derr := sounds.DecodeFile(tmp)
		os.Remove(tmp)
		if derr != nil {
			return nil, fmt.Errorf("%s: не удалось прочитать аудио: %v", fh.Filename, derr)
		}
		if err := os.WriteFile(filepath.Join(dir, name+ext), data, 0o644); err != nil {
			return nil, err
		}
		saved = append(saved, name)
		log.Printf("Добавлен звук: %s", name)
	}
	if err := s.App.Reload(); err != nil {
		return nil, err
	}
	st, _ := s.state(r)
	return map[string]any{"saved": saved, "state": st}, nil
}

func (s *Server) deleteSound(r *http.Request) (any, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	found := false
	for _, inf := range s.App.Sounds() {
		if inf.ID != req.ID {
			continue
		}
		found = true
		if inf.Source == "builtin" {
			return nil, fmt.Errorf("встроенный звук удалить нельзя")
		}
		if inf.Source == "file" {
			for _, f := range inf.Files {
				p := f
				if !filepath.IsAbs(p) {
					p = filepath.Join(s.App.BaseDir, p)
				}
				os.Remove(p)
			}
		}
	}
	if !found {
		return nil, fmt.Errorf("звук %q не найден", req.ID)
	}
	err := s.App.UpdateConfig(func(c *config.Config) {
		delete(c.Sounds, req.ID)
		for k, v := range c.Hotkeys {
			if v == "sound:"+req.ID || strings.HasPrefix(v, "sound:"+req.ID+"@") {
				delete(c.Hotkeys, k)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	log.Printf("Звук удалён: %s", req.ID)
	return s.state(r)
}

func (s *Server) autoSetup(r *http.Request) (any, error) {
	var req struct {
		AllowSteamRestart bool `json:"allow_steam_restart"`
	}
	decode(r, &req)
	rep := s.App.AutoSetup(req.AllowSteamRestart)
	return map[string]any{"report": rep, "setup": s.App.SetupStatus()}, nil
}

func (s *Server) fix(r *http.Request) (any, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if err := s.App.Fix(req.ID); err != nil {
		return nil, err
	}
	return s.App.SetupStatus(), nil
}

func (s *Server) autostart(r *http.Request) (any, error) {
	var req struct {
		On bool `json:"on"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return nil, winapi.SetAutostart(req.On)
}

func (s *Server) pauseHotkeys(r *http.Request) (any, error) {
	var req struct {
		On bool `json:"on"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	s.App.PauseHotkeys(req.On)
	return nil, nil
}

func (s *Server) open(r *http.Request) (any, error) {
	var req struct {
		What string `json:"what"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	switch req.What {
	case "sounds":
		os.MkdirAll(s.App.SoundsDir(), 0o755)
		return nil, winapi.OpenURL(s.App.SoundsDir())
	case "config":
		return nil, winapi.OpenURL(s.App.BaseDir)
	case "cable":
		return nil, winapi.OpenURL("https://vb-audio.com/Cable/")
	case "repo":
		return nil, winapi.OpenURL("https://github.com/qwkejkqwje1/dota-voicebox")
	case "discord_voice":
		return nil, winapi.OpenURL("discord://-/settings/voice")
	}
	return nil, fmt.Errorf("неизвестно: %s", req.What)
}

func (s *Server) quit(*http.Request) (any, error) {
	if s.OnQuit != nil {
		go func() { time.Sleep(200 * time.Millisecond); s.OnQuit() }()
	}
	return nil, nil
}

// testPreset — временно включить пресет из редактора (без сохранения).
func (s *Server) testPreset(r *http.Request) (any, error) {
	var req struct {
		Name  string     `json:"name"`
		Specs []dsp.Spec `json:"specs"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	c, err := dsp.BuildChain(req.Name, req.Specs, sounds.SampleRate)
	if err != nil {
		return nil, err
	}
	s.Engine.SetChain(c)
	return nil, nil
}

// ---------- скрипты ----------

func (s *Server) scriptsList(*http.Request) (any, error) {
	return map[string]any{"scripts": s.App.Scripts().List()}, nil
}

func (s *Server) scriptCode(r *http.Request) (any, error) {
	code, err := s.App.Scripts().Read(r.URL.Query().Get("name"))
	if err != nil {
		return nil, err
	}
	return map[string]string{"code": code}, nil
}

type compileInfo struct {
	Error string `json:"error,omitempty"`
	Line  int    `json:"line,omitempty"`
}

func ci(err error) compileInfo {
	if err == nil {
		return compileInfo{}
	}
	return compileInfo{Error: err.Error(), Line: script.ErrLine(err)}
}

func (s *Server) scriptSave(r *http.Request) (any, error) {
	var req struct {
		Name   string `json:"name"`
		Code   string `json:"code"`
		Enable *bool  `json:"enable"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	m := s.App.Scripts()
	cerr, err := m.Save(req.Name, req.Code)
	if err != nil {
		return nil, err
	}
	if req.Enable != nil {
		if err := s.setEnabled(req.Name, *req.Enable); err != nil {
			return nil, err
		}
	} else {
		m.Sync(s.App.Config().Scripts)
	}
	return map[string]any{"compile": ci(cerr), "scripts": m.List()}, nil
}

func (s *Server) setEnabled(name string, on bool) error {
	return s.App.UpdateConfig(func(c *config.Config) {
		if c.Scripts == nil {
			c.Scripts = map[string]bool{}
		}
		c.Scripts[name] = on
	})
}

func (s *Server) scriptEnable(r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
		On   bool   `json:"on"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if !script.ValidName(req.Name) {
		return nil, fmt.Errorf("плохое имя скрипта")
	}
	if err := s.setEnabled(req.Name, req.On); err != nil {
		return nil, err
	}
	return map[string]any{"scripts": s.App.Scripts().List()}, nil
}

func (s *Server) scriptDelete(r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if err := s.App.Scripts().Delete(req.Name); err != nil {
		return nil, err
	}
	s.App.UpdateConfig(func(c *config.Config) { delete(c.Scripts, req.Name) })
	return map[string]any{"scripts": s.App.Scripts().List()}, nil
}

func (s *Server) scriptRun(r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	err := s.App.Scripts().Run(req.Name)
	return map[string]any{"compile": ci(err), "scripts": s.App.Scripts().List()}, nil
}

func (s *Server) scriptCheck(r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return ci(script.Check(req.Name, req.Code)), nil
}

// ---------- тесты и время ----------

func (s *Server) runTest(r *http.Request) (any, error) {
	var req struct {
		ID     string  `json:"id"`
		Sec    float64 `json:"sec"`
		Preset string  `json:"preset"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.App.RunTest(req.ID, req.Sec, req.Preset)
}

func (s *Server) setClock(r *http.Request) (any, error) {
	var req struct {
		Mode string `json:"mode"` // horn | sync | clear
		Time string `json:"time"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	switch req.Mode {
	case "horn":
		s.App.SetManualClock(0)
	case "sync":
		sec, err := script.ParseTime(req.Time)
		if err != nil {
			return nil, err
		}
		s.App.SetManualClock(sec)
	case "clear":
		s.App.Clock().ClearManual()
	default:
		return nil, fmt.Errorf("mode: horn|sync|clear")
	}
	return nil, nil
}

// ---------- обновления ----------

func (s *Server) updateCheck(*http.Request) (any, error) {
	s.Updater.Check()
	return s.Updater.Status(), nil
}

func (s *Server) updateInstall(*http.Request) (any, error) {
	st := s.Updater.Status()
	if st.State != "ready" {
		if !st.Available {
			if _, err := s.Updater.Check(); err != nil {
				return nil, err
			}
			if !s.Updater.Status().Available {
				return nil, fmt.Errorf("у вас последняя версия")
			}
		}
		if err := s.Updater.Download(); err != nil {
			return nil, err
		}
	}
	if err := s.Updater.Install(s.RestartArgs); err != nil {
		return nil, err
	}
	if s.OnQuit != nil {
		go func() { time.Sleep(300 * time.Millisecond); s.OnQuit() }()
	}
	return s.Updater.Status(), nil
}

func (s *Server) updateRollback(*http.Request) (any, error) {
	if err := s.Updater.Rollback(s.RestartArgs); err != nil {
		return nil, err
	}
	log.Print("Откат на прошлую версию, перезапуск…")
	if s.OnQuit != nil {
		go func() { time.Sleep(300 * time.Millisecond); s.OnQuit() }()
	}
	return nil, nil
}

func (s *Server) restoreBackup(r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if err := config.Restore(s.App.ConfigPath, req.Name); err != nil {
		return nil, err
	}
	if err := s.App.Reload(); err != nil {
		return nil, err
	}
	log.Printf("Конфиг восстановлен из копии %s", req.Name)
	return s.state(r)
}
