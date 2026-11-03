package ui

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// API для нового интерфейса: плитки, эффекты голоса, быстрое меню, модуль Dota, синтез речи.

func (s *Server) api2() map[string]func(*http.Request) (any, error) {
	return map[string]func(*http.Request) (any, error){
		"POST /api/play":             s.play,
		"POST /api/stop":             s.stop,
		"POST /api/sounds/import":    s.importSounds,
		"POST /api/sounds/phrase":    s.addPhrase,
		"POST /api/sounds/builtin":   s.addBuiltin,
		"POST /api/sounds/update":    s.updateSound,
		"POST /api/sounds/delete":    s.deleteSound,
		"POST /api/sounds/order":     s.soundOrder,
		"POST /api/preset":           s.setPreset,
		"POST /api/preset/params":    s.presetParams,
		"POST /api/preset/reset":     s.presetReset,
		"POST /api/hotkey":           s.bindHotkey,
		"POST /api/menu":             s.setMenu,
		"POST /api/menu/preview":     s.menuPreview,
		"POST /api/wheel":            s.setWheel,
		"POST /api/wheel/preview":    s.wheelPreview,
		"GET /api/remote":            s.remoteStatus,
		"POST /api/remote":           s.setRemote,
		"POST /api/remote/reset":     s.remoteReset,
		"POST /api/remote/firewall":  s.remoteFirewall,
		"POST /api/remote/open":      s.remoteOpen,
		"POST /api/dota":             s.setDota,
		"POST /api/dota/test":        s.testReminder,
		"GET /api/tts/voices":        s.ttsVoices,
		"POST /api/tts/say":          s.ttsSay,
		"POST /api/tts/retry":        func(*http.Request) (any, error) { go s.App.RetrySpeech(); return nil, nil },
		"POST /api/emoji/missing":    s.emojiMissing,
		"POST /api/emoji":            s.emojiSave,
		"POST /api/settings":         s.settings,
		"GET /api/devices":           s.devices,
		"POST /api/action/sync":      s.actionSync,
		"GET /api/status":            func(*http.Request) (any, error) { return s.App.Status(), nil },
		"GET /api/menu":              func(*http.Request) (any, error) { return s.App.Menu(), nil },
		"POST /api/sounds/validate":  s.validateSound,
		"POST /api/sets/save":        s.saveSet,
		"POST /api/sets/delete":      s.deleteSet,
		"POST /api/sets/order":       s.setOrder,
		"POST /api/sets/active":      s.activeSet,
		"POST /api/bg/preview":       s.bgPreview,
		"GET /api/rec":               func(*http.Request) (any, error) { return s.App.RecStatus(), nil },
		"POST /api/rec/start":        s.recStart,
		"POST /api/rec/stop":         func(*http.Request) (any, error) { return s.App.RecStop() },
		"POST /api/rec/play":         s.recPlay,
		"POST /api/rec/save":         s.recSave,
		"POST /api/rec/cancel":       func(*http.Request) (any, error) { s.App.RecDiscard(); return nil, nil },
		"GET /api/sounds/wave":       func(r *http.Request) (any, error) { return s.App.SoundWave(r.URL.Query().Get("id")) },
		"POST /api/sounds/wave/play": s.wavePlay,
		"POST /api/sounds/wave/done": func(*http.Request) (any, error) { s.App.WaveDone(); return nil, nil },
		"POST /api/sounds/trim":      s.trimSound,
		"POST /api/dota/events":      s.dotaEvent,
	}
}

// rangeReq — кусок звука: с какой секунды и до какой (0 — до конца).
type rangeReq struct {
	ID     string  `json:"id"`
	Start  float64 `json:"start"`
	End    float64 `json:"end"`
	Effect bool    `json:"effect"` // запись — с эффектом голоса
	Name   string  `json:"name"`
}

func (s *Server) recStart(*http.Request) (any, error) {
	if err := s.App.RecStart(); err != nil {
		return nil, err
	}
	return s.App.RecStatus(), nil
}

func (s *Server) recPlay(r *http.Request) (any, error) {
	var req rangeReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return nil, s.App.RecPlay(req.Start, req.End, req.Effect)
}

func (s *Server) recSave(r *http.Request) (any, error) {
	var req rangeReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	id, err := s.App.RecSave(req.Name, req.Start, req.End, req.Effect)
	if err != nil {
		return nil, err
	}
	log.Printf("Записан звук: %s", id)
	st, _ := s.state(r)
	return map[string]any{"id": id, "state": st}, nil
}

func (s *Server) wavePlay(r *http.Request) (any, error) {
	var req rangeReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return nil, s.App.PlayRange(req.ID, req.Start, req.End)
}

func (s *Server) trimSound(r *http.Request) (any, error) {
	var req rangeReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.TrimSound(req.ID, req.Start, req.End))
}

func (s *Server) dotaEvent(r *http.Request) (any, error) {
	var req struct {
		Event string `json:"event"`
		Sound string `json:"sound"`
		To    string `json:"to"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.SetEventSound(req.Event, req.Sound, req.To))
}

// soundView — плитка для интерфейса.
type soundView struct {
	sounds.Info
	Title  string `json:"title"`
	Hotkey string `json:"hotkey,omitempty"`
	InMenu bool   `json:"in_menu"`
	Text   string `json:"text,omitempty"` // фраза синтеза речи
	Kind   string `json:"kind"`           // file | phrase | builtin | mixed
	OnPad  bool   `json:"on_pad"`
}

type presetView struct {
	dsp.Preset
	Values map[string]float64 `json:"values,omitempty"`
	Custom bool               `json:"custom,omitempty"` // свой пресет из voice_presets
	Hotkey string             `json:"hotkey,omitempty"`
}

func soundKind(files []string) (kind, text string) {
	for _, f := range files {
		k := "file"
		switch {
		case strings.HasPrefix(f, "tts:"):
			k, text = "phrase", strings.TrimPrefix(f, "tts:")
		case strings.HasPrefix(f, "builtin:"):
			k = "builtin"
		}
		if kind != "" && kind != k {
			kind = "mixed"
		} else {
			kind = k
		}
	}
	if kind == "" {
		kind = "file"
	}
	return kind, text
}

func (s *Server) state(*http.Request) (any, error) {
	cfg := s.App.Config()
	hk := s.App.HotkeyMap()
	menu := s.App.Menu()
	inMenu := map[string]bool{}
	for _, it := range menu.Sounds {
		inMenu[it.ID] = true
	}
	pad := s.App.PadOrder()
	onPad := map[string]bool{}
	for _, id := range pad {
		onPad[id] = true
	}
	var list []soundView
	for _, inf := range s.App.Sounds() {
		v := soundView{Info: inf, Hotkey: hk["sound:"+inf.ID], InMenu: inMenu[inf.ID], OnPad: onPad[inf.ID]}
		v.Kind, v.Text = soundKind(inf.Files)
		v.Title = inf.Label
		if v.Title == "" {
			v.Title = app.DisplayLabel(&sounds.Sound{ID: inf.ID})
		}
		if v.Emoji == "" {
			v.Emoji = "🔊"
		}
		list = append(list, v)
	}
	var presets []presetView
	for _, id := range s.App.PresetNames() {
		pv := presetView{Hotkey: hk["preset:"+id]}
		if p, ok := dsp.FindPreset(id); ok {
			pv.Preset = p
			pv.Values = p.Values(cfg.PresetParams[id])
		} else {
			pv.Preset = dsp.Preset{ID: id, Title: id, Emoji: "🎛️", Desc: "Свой пресет из config.json", Group: "custom"}
		}
		if _, ok := cfg.VoicePresets[id]; ok {
			pv.Custom = true
		}
		presets = append(presets, pv)
	}
	capture, playback, derr := audio.Devices()
	cable, cableRec := s.App.CableNames(capture, playback)
	errStr := ""
	if derr != nil {
		errStr = derr.Error()
	}
	return map[string]any{
		"version":    s.App.Version,
		"demo":       audio.Demo,
		"config":     cfg,
		"sounds":     list,
		"pad":        pad,
		"hotkeys":    hk,
		"presets":    presets,
		"builtins":   sounds.BuiltinInfos(),
		"menu":       menu,
		"wheel":      s.App.Wheel(),
		"palette":    app.Palette,
		"devices":    map[string]any{"capture": capture, "playback": playback, "error": errStr, "cable": cable, "cable_rec": cableRec},
		"events":     gsiEvents,
		"logs":       s.Logs.Lines(),
		"status":     s.App.Status(),
		"sounds_dir": s.App.SoundsDir(),
		"autostart":  winapi.Autostart(),
		"reminders":  reminderViews(cfg),
		"voice": map[string]any{"timbres": dsp.Timbres, "eq_bands": dsp.EQBands, "eq_max": dsp.EQMax,
			"ambiences": dsp.Ambiences},
		"games":       app.KnownGames,
		"recent_apps": s.App.RecentApps(),
	}, nil
}

type reminderView struct {
	config.Reminder
	Phrase string `json:"phrase"`
}

func reminderViews(cfg *config.Config) []reminderView {
	out := []reminderView{}
	for _, r := range cfg.Dota.Reminders {
		out = append(out, reminderView{Reminder: r, Phrase: app.ReminderPhrase(r)})
	}
	return out
}

// ok — ответ «готово» с новым состоянием (интерфейс сразу перерисовывается).
func (s *Server) ok(r *http.Request, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return s.state(r)
}

type idReq struct {
	ID string `json:"id"`
}

func (s *Server) play(r *http.Request) (any, error) {
	var req struct {
		ID    string  `json:"id"`
		To    string  `json:"to"` // "" — как настроено; monitor — только мне
		Stop  bool    `json:"stop"`
		Level float64 `json:"volume"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if req.Stop {
		s.App.StopSound(req.ID)
		return nil, nil
	}
	ref := req.ID
	if req.To != "" {
		ref += "@" + req.To
	}
	vol := req.Level
	if vol <= 0 {
		vol = 1
	}
	return nil, s.App.PlayOpts(ref, vol)
}

func (s *Server) stop(*http.Request) (any, error) {
	s.App.StopAll()
	return nil, nil
}

func (s *Server) importSounds(r *http.Request) (any, error) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return nil, err
	}
	var ids []string
	var errs []string
	for _, fh := range r.MultipartForm.File["files"] {
		f, err := fh.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(f, 48<<20))
		f.Close()
		if err != nil {
			return nil, err
		}
		name := fh.Filename
		if n := r.FormValue("name"); n != "" && len(r.MultipartForm.File["files"]) == 1 {
			name = n
		}
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		id, err := s.App.ImportSound(strings.TrimSuffix(name, filepath.Ext(name)), data, ext)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", fh.Filename, err))
			continue
		}
		log.Printf("Добавлен звук: %s", id)
		ids = append(ids, id)
	}
	st, _ := s.state(r)
	if len(ids) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return map[string]any{"ids": ids, "errors": errs, "state": st}, nil
}

// validateSound — проверить файл до добавления (интерфейс решает, нужна ли перекодировка в WAV).
func (s *Server) validateSound(r *http.Request) (any, error) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 48<<20))
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(r.URL.Query().Get("ext"))
	var clip sounds.Clip
	switch ext {
	case ".wav":
		clip, err = sounds.DecodeWAV(data)
	case ".mp3":
		clip, err = sounds.DecodeMP3(data)
	default:
		return map[string]any{"ok": false}, nil
	}
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	return map[string]any{"ok": true, "duration": float64(len(clip)) / sounds.SampleRate}, nil
}

func (s *Server) addPhrase(r *http.Request) (any, error) {
	var req struct {
		Text  string `json:"text"`
		Emoji string `json:"emoji"`
		Dota  bool   `json:"dota"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	id, err := s.App.AddPhraseOpts(req.Text, req.Emoji, req.Dota)
	if err != nil {
		return nil, err
	}
	st, _ := s.state(r)
	return map[string]any{"id": id, "state": st}, nil
}

func (s *Server) addBuiltin(r *http.Request) (any, error) {
	var req struct {
		Name string `json:"name"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	id, err := s.App.AddBuiltin(req.Name)
	if err != nil {
		return nil, err
	}
	st, _ := s.state(r)
	return map[string]any{"id": id, "state": st}, nil
}

func (s *Server) updateSound(r *http.Request) (any, error) {
	var req struct {
		ID   string        `json:"id"`
		Edit app.SoundEdit `json:"edit"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.UpdateSound(req.ID, req.Edit))
}

func (s *Server) deleteSound(r *http.Request) (any, error) {
	var req idReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	err := s.App.DeleteSound(req.ID)
	if err == nil {
		log.Printf("Звук удалён: %s", req.ID)
	}
	return s.ok(r, err)
}

func (s *Server) soundOrder(r *http.Request) (any, error) {
	var req struct {
		Order []string `json:"order"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return nil, s.App.SetPad(req.Order)
}

func (s *Server) setPreset(r *http.Request) (any, error) {
	var req idReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	known := false
	for _, n := range s.App.PresetNames() {
		known = known || n == req.ID
	}
	if !known {
		return nil, fmt.Errorf("нет эффекта %q", req.ID)
	}
	s.App.SetPreset(req.ID, true)
	return nil, nil
}

func (s *Server) presetParams(r *http.Request) (any, error) {
	var req struct {
		ID     string             `json:"id"`
		Values map[string]float64 `json:"values"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if err := s.App.SetPresetParams(req.ID, req.Values); err != nil {
		return nil, err
	}
	p, _ := dsp.FindPreset(req.ID)
	return map[string]any{"values": p.Values(s.App.Config().PresetParams[req.ID])}, nil
}

func (s *Server) presetReset(r *http.Request) (any, error) {
	var req idReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.ResetPresetParams(req.ID))
}

func (s *Server) bindHotkey(r *http.Request) (any, error) {
	var req struct {
		Action string `json:"action"`
		Key    string `json:"key"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if req.Action == "" {
		return nil, fmt.Errorf("не указано действие")
	}
	return s.ok(r, s.App.BindHotkey(req.Action, req.Key))
}

func (s *Server) setMenu(r *http.Request) (any, error) {
	var req struct {
		Menu config.QuickMenu `json:"menu"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	enabledBefore := s.App.Config().QuickMenu.Enabled
	var err error
	if enabledBefore != req.Menu.Enabled { // горячая клавиша меню включается/выключается — нужен Reload
		err = s.App.UpdateConfig(func(c *config.Config) { c.QuickMenu = req.Menu })
	} else {
		err = s.App.SetQuickMenu(req.Menu)
	}
	return s.ok(r, err)
}

func (s *Server) menuPreview(*http.Request) (any, error) {
	if s.OnMenuPreview == nil {
		return nil, fmt.Errorf("быстрое меню работает только в Windows")
	}
	s.OnMenuPreview()
	return nil, nil
}

func (s *Server) setWheel(r *http.Request) (any, error) {
	var req struct {
		Wheel config.Wheel `json:"wheel"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.SetWheel(req.Wheel))
}

func (s *Server) wheelPreview(*http.Request) (any, error) {
	if s.OnWheelPreview == nil {
		return nil, fmt.Errorf("колесо звуков работает только в Windows")
	}
	s.OnWheelPreview()
	return nil, nil
}

func (s *Server) remoteStatus(*http.Request) (any, error) {
	if s.Remote == nil {
		return nil, fmt.Errorf("пульт недоступен")
	}
	return s.Remote.Status(), nil
}

func (s *Server) setRemote(r *http.Request) (any, error) {
	var req struct {
		Remote config.Remote `json:"remote"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	rm := req.Remote
	cur := s.App.Config()
	if rm.Port != 0 && (rm.Port < 1024 || rm.Port > 65535) {
		return nil, fmt.Errorf("порт пульта — число от 1024 до 65535 (0 — автоматически)")
	}
	ui := cur.UI.Port
	if ui == 0 {
		ui = 3001
	}
	if rm.Port == ui {
		return nil, fmt.Errorf("порт %d уже занят самим интерфейсом VoiceBox", ui)
	}
	rm.Key = cur.Remote.Key // ключ меняется только кнопкой «Новая ссылка»
	if err := s.App.SetRemote(rm); err != nil {
		return nil, err
	}
	return s.remoteStatus(r)
}

func (s *Server) remoteReset(r *http.Request) (any, error) {
	err := s.App.SaveRemoteKey(config.NewRemoteKey())
	if err != nil {
		return nil, err
	}
	if s.Remote != nil {
		s.Remote.ForgetDevices()
	}
	log.Print("Пульт с телефона: новая ссылка, старые больше не работают")
	return s.remoteStatus(r)
}

func (s *Server) remoteFirewall(r *http.Request) (any, error) {
	c := s.App.Config()
	if err := winapi.AllowFirewall(s.Exe, c.RemotePort()); err != nil {
		return nil, err
	}
	log.Printf("Брандмауэр: разрешён порт %d для пульта с телефона (домашние сети, только локальные устройства)", c.RemotePort())
	return s.remoteStatus(r)
}

func (s *Server) remoteOpen(r *http.Request) (any, error) {
	var req struct {
		What string `json:"what"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	switch req.What {
	case "firewall_apps":
		return nil, winapi.OpenFirewallApps()
	case "network":
		return nil, winapi.OpenNetworkSettings()
	}
	return nil, fmt.Errorf("неизвестно: %s", req.What)
}

func (s *Server) setDota(r *http.Request) (any, error) {
	var req struct {
		Dota config.Dota `json:"dota"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.SetDota(req.Dota))
}

func (s *Server) testReminder(r *http.Request) (any, error) {
	var req struct {
		Reminder config.Reminder `json:"reminder"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return nil, s.App.TestReminder(req.Reminder)
}

func (s *Server) ttsVoices(*http.Request) (any, error) {
	v, err := s.App.Voices()
	if err != nil {
		return map[string]any{"voices": []winapi.Voice{}, "error": err.Error()}, nil
	}
	return map[string]any{"voices": v}, nil
}

func (s *Server) ttsSay(r *http.Request) (any, error) {
	var req struct {
		Text string `json:"text"`
		To   string `json:"to"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	bus := sounds.ParseBus(req.To)
	if req.To == "" {
		bus = sounds.ParseBus("monitor")
	}
	return nil, s.App.Say(req.Text, bus)
}

func (s *Server) emojiMissing(r *http.Request) (any, error) {
	var req struct {
		List []string `json:"list"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	out := []string{}
	if s.Emoji == nil {
		return map[string]any{"missing": out}, nil
	}
	seen := map[string]bool{}
	for _, e := range req.List {
		if e == "" || seen[e] || len(req.List) > 500 {
			continue
		}
		seen[e] = true
		if !s.Emoji.Has(e) {
			out = append(out, e)
		}
	}
	return map[string]any{"missing": out}, nil
}

func (s *Server) emojiSave(r *http.Request) (any, error) {
	if s.Emoji == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 300<<10))
	if err != nil {
		return nil, err
	}
	return nil, s.Emoji.Save(r.URL.Query().Get("e"), data)
}

// quietSettings — настройки, которым не нужна полная перезагрузка (улучшение голоса, наборы):
// ползунки эквалайзера и громкости фона применяются сразу, фон не начинается заново.
var quietSettings = map[string]bool{"denoise": true, "denoise_level": true, "timbre": true, "eq": true, "smooth": true,
	"bg_id": true, "bg_volume": true, "bg_only_talking": true, "auto_set": true}

// settings — изменение отдельных настроек без пересылки всего конфига.
func (s *Server) settings(r *http.Request) (any, error) {
	var req map[string]any
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	var bad []string
	update := s.App.UpdateEnhance
	for k := range req {
		if !quietSettings[k] {
			update = s.App.UpdateConfig
		}
	}
	err := update(func(c *config.Config) {
		for k, v := range req {
			if !applySetting(c, k, v) {
				bad = append(bad, k)
			}
		}
	})
	if len(bad) > 0 {
		log.Printf("настройки: неизвестные поля %v", bad)
	}
	return s.ok(r, err)
}

func num(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func applySetting(c *config.Config, k string, v any) bool {
	str, isStr := v.(string)
	b, isBool := v.(bool)
	f, isNum := num(v)
	switch {
	case k == "mic" && isStr:
		c.Devices.Mic = str
	case k == "voice_out" && isStr:
		c.Devices.VoiceOut = str
	case k == "monitor" && isStr:
		c.Devices.Monitor = str
	case k == "cable_rec" && isStr:
		c.Devices.CableRec = str
	case k == "mic_gain" && isNum:
		c.MicGain = clamp(f, 0, 4)
	case k == "mic_mute" && isBool:
		c.Mic.Mute = b
	case k == "gate_db" && isNum:
		c.Mic.GateDB = clamp(f, -90, 0)
	case k == "monitor_volume" && isNum:
		c.MonitorVolume = clamp(f, 0, 2)
	case k == "sfx_volume" && isNum:
		c.SfxVolume = clamp(f, 0, 2)
	case k == "ducking" && isNum:
		c.Ducking = clamp(f, 0, 1)
	case k == "fx_on_sounds" && isBool:
		c.FxOnSounds = b
	case k == "normalize" && isBool:
		c.Normalize = b
	case k == "tts_voice" && isStr:
		c.TTSVoice = str
	case k == "tts_rate" && isNum:
		c.TTSRate = int(clamp(f, -10, 10))
	case k == "talk_mode" && isStr:
		switch str {
		case "always", "ptt", "toggle":
			c.Talk.Mode = str
		}
	case k == "talk_key" && isStr:
		c.Talk.Key = str
	case k == "talk_release_ms" && isNum:
		c.Talk.ReleaseMS = int(clamp(f, 0, 2000))
	case k == "talk_press_game" && isBool:
		c.Talk.PressGame = b
	case k == "ptt_key" && isStr:
		c.PTT.Key = str
	case k == "ptt_auto" && isBool:
		c.PTT.Auto = b
	case k == "ptt_auto_detect" && isBool:
		c.PTT.AutoDetect = b
	case k == "ptt_lead_ms" && isNum:
		c.PTT.LeadMS = int(clamp(f, 0, 1000))
	case k == "ptt_tail_ms" && isNum:
		c.PTT.TailMS = int(clamp(f, 0, 2000))
	case k == "low_hp_percent" && isNum:
		c.GSI.LowHP = int(clamp(f, 0, 90))
	case k == "keep_running" && isBool:
		c.UI.KeepRunningOnClose = b
	case k == "open_on_start" && isBool:
		c.UI.OpenOnStart = b
	case k == "update_auto_check" && isBool:
		c.Update.AutoCheck = b
	case k == "update_auto_install" && isBool:
		c.Update.AutoInstall = b
	case k == "setup_done" && isBool:
		c.SetupDone = b
	case k == "gsi_enabled" && isBool:
		c.GSI.Enabled = b
	case k == "denoise" && isBool:
		c.Mic.Denoise = b
	case k == "denoise_level" && isNum:
		c.Mic.DenoiseLevel = clamp(f, 0.3, 1)
	case k == "timbre" && isStr:
		if _, ok := dsp.FindTimbre(str); ok || str == "" || str == "custom" {
			c.Mic.Timbre = str
		}
	case k == "eq":
		list, ok := v.([]any)
		if !ok {
			return false
		}
		eq := make([]float64, len(dsp.EQBands))
		for i := range eq {
			if i < len(list) {
				x, _ := num(list[i])
				eq[i] = clamp(x, -dsp.EQMax, dsp.EQMax)
			}
		}
		c.Mic.EQ = eq
	case k == "smooth" && isNum:
		c.Mic.Smooth = clamp(f, 0, 1)
	case k == "bg_id" && isStr:
		if c.Background.ID != "" && c.Background.ID != str {
			c.Background.Last = c.Background.ID
		}
		c.Background.ID = str
	case k == "bg_volume" && isNum:
		c.Background.Volume = clamp(f, 0, 1)
	case k == "bg_only_talking" && isBool:
		c.Background.OnlyTalking = b
	case k == "auto_set" && isBool:
		c.AutoSet = b
	default:
		return false
	}
	return true
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (s *Server) devices(*http.Request) (any, error) {
	capture, playback, derr := audio.Devices()
	cable, cableRec := s.App.CableNames(capture, playback)
	errStr := ""
	if derr != nil {
		errStr = derr.Error()
	}
	return map[string]any{"capture": capture, "playback": playback, "error": errStr, "cable": cable, "cable_rec": cableRec}, nil
}

// actionSync — действие с ответом-состоянием (кнопки интерфейса, которые меняют настройки).
func (s *Server) actionSync(r *http.Request) (any, error) {
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	s.App.Do(req.Action)
	return s.state(r)
}

// ---------- наборы звуков ----------

func (s *Server) saveSet(r *http.Request) (any, error) {
	var req struct {
		Set config.SoundSet `json:"set"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	id, err := s.App.SaveSet(req.Set)
	if err != nil {
		return nil, err
	}
	st, err := s.state(r)
	return map[string]any{"id": id, "state": st}, err
}

func (s *Server) deleteSet(r *http.Request) (any, error) {
	var req idReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.DeleteSet(req.ID))
}

func (s *Server) setOrder(r *http.Request) (any, error) {
	var req struct {
		ID    string   `json:"id"`
		Order []string `json:"order"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.SetSetOrder(req.ID, req.Order))
}

func (s *Server) activeSet(r *http.Request) (any, error) {
	var req idReq
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return s.ok(r, s.App.SetActiveSet(req.ID, false))
}

// bgPreview — послушать фон в наушниках.
func (s *Server) bgPreview(r *http.Request) (any, error) {
	var req struct {
		ID     string  `json:"id"`
		Volume float64 `json:"volume"`
	}
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	return nil, s.App.PreviewBackground(req.ID, req.Volume)
}
