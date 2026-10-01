// Package app связывает всё вместе: горячие клавиши → действия, GSI → события/таймеры,
// автонажатие PTT, пресеты голоса, автонастройка и горячая перезагрузка конфига.
package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/timers"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// Output — то, что нужно приложению от аудио-движка (для тестов подменяется).
type Output interface {
	Voice() *audio.Mixer
	Monitor() *audio.Mixer
	SetChain(*dsp.Chain)
	ToggleMicMonitor() bool
	MicMonitor() bool
	Running() bool
	Start(audio.Options) error
	SetMix(micGain, ducking, sfxVol, monVol float64, fxOnSounds bool)
	Status() audio.Status
}

type App struct {
	ConfigPath string
	BaseDir    string
	Version    string

	mu      sync.Mutex
	cfg     *config.Config
	lib     *sounds.Library
	out     Output
	hk      *winapi.Hotkeys
	combos  []keys.Combo
	timers  *timers.Engine
	presets map[string][]dsp.Spec
	order   []string
	preset  string
	actions map[string]string
	pttVK   uint32
	devOpts *audio.Options

	gsiMu     sync.Mutex
	prevState *gsi.State
	lastGSI   time.Time

	pttOwned  atomic.Bool
	pending   atomic.Int32
	lastVoice atomic.Int64
	stamp     string
	hkPaused  atomic.Bool

	// OnShowUI — вызывается действием "ui" (открыть окно).
	OnShowUI func()
}

func New(configPath string, out Output) *App {
	return &App{ConfigPath: configPath, BaseDir: filepath.Dir(configPath), out: out, timers: timers.New(nil)}
}

// Config возвращает копию текущего конфига.
func (a *App) Config() *config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, _ := json.Marshal(a.cfg)
	var c config.Config
	json.Unmarshal(b, &c)
	return &c
}

// SaveConfig сохраняет конфиг на диск и применяет его.
func (a *App) SaveConfig(c *config.Config) error {
	// проверяем пресеты до сохранения, чтобы не записать битый конфиг
	for name, specs := range c.VoicePresets {
		if _, err := dsp.BuildChain(name, specs, sounds.SampleRate); err != nil {
			return err
		}
	}
	for k := range c.Hotkeys {
		if _, err := keys.Parse(k); err != nil {
			return err
		}
	}
	if c.PTT.Key != "" {
		if _, err := keys.Parse(c.PTT.Key); err != nil {
			return fmt.Errorf("ptt: %w", err)
		}
	}
	if err := config.Save(a.ConfigPath, c); err != nil {
		return err
	}
	err := a.Reload()
	a.stamp = a.computeStamp()
	return err
}

// UpdateConfig — изменить конфиг функцией и сохранить.
func (a *App) UpdateConfig(f func(c *config.Config)) error {
	c := a.Config()
	f(c)
	return a.SaveConfig(c)
}

// Reload (пере)читывает конфиг, звуки, пресеты, таймеры, горячие клавиши и устройства.
func (a *App) Reload() error {
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		return err
	}
	tts := winapi.NewTTS(filepath.Join(a.BaseDir, "cache"), cfg.TTSRate)
	lib := sounds.Load(a.BaseDir, cfg.SoundsDir, cfg.Sounds, tts, cfg.Normalize)

	presets := map[string][]dsp.Spec{}
	for k, v := range dsp.BuiltinPresets {
		presets[k] = v
	}
	var custom []string
	for k, v := range cfg.VoicePresets {
		if _, err := dsp.BuildChain(k, v, sounds.SampleRate); err != nil {
			log.Printf("пресет %s пропущен: %v", k, err)
			continue
		}
		if _, builtin := dsp.BuiltinPresets[k]; !builtin {
			custom = append(custom, k)
		}
		presets[k] = v
	}
	sort.Strings(custom)
	order := append(append([]string{}, dsp.PresetOrder...), custom...)

	var pttVK uint32
	if cfg.PTT.Key != "" {
		if c, err := keys.Parse(cfg.PTT.Key); err != nil {
			log.Printf("ptt.key: %v", err)
		} else {
			pttVK = c.VK
		}
	}

	actions := map[string]string{}
	var combos []keys.Combo
	for k, act := range cfg.Hotkeys {
		c, err := keys.Parse(k)
		if err != nil {
			log.Printf("hotkeys: %v", err)
			continue
		}
		actions[c.Text] = act
		combos = append(combos, c)
	}

	a.mu.Lock()
	first := a.cfg == nil
	a.cfg, a.lib, a.presets, a.order, a.actions, a.pttVK, a.combos = cfg, lib, presets, order, actions, pttVK, combos
	a.timers.SetTimers(cfg.Timers)
	a.mu.Unlock()

	a.out.SetMix(cfg.MicGain, cfg.Ducking, or1(cfg.SfxVolume), cfg.MonitorVolume, cfg.FxOnSounds)
	a.applyHotkeys()

	// устройства: перезапуск движка только если выбор изменился
	opts := audio.Options{MicDevice: cfg.Devices.Mic, VoiceOut: cfg.Devices.VoiceOut, MonitorOut: cfg.Devices.Monitor}
	a.mu.Lock()
	changed := a.devOpts == nil || *a.devOpts != opts
	a.devOpts = &opts
	a.mu.Unlock()
	if changed {
		if err := a.out.Start(opts); err != nil {
			log.Printf("Аудио: %v", err)
		}
	}

	if first {
		p := cfg.StartPreset
		if p == "" {
			p = "clean"
		}
		a.SetPreset(p, false)
	} else {
		a.SetPreset(a.Preset(), false)
	}
	log.Printf("Конфиг применён: %d звуков, %d горячих клавиш, %d пресетов", len(lib.IDs()), len(combos), len(presets))
	return nil
}

func or1(v float64) float64 {
	if v == 0 {
		return 1
	}
	return v
}

func (a *App) applyHotkeys() {
	if a.hk == nil {
		return
	}
	var combos []keys.Combo
	if !a.hkPaused.Load() {
		a.mu.Lock()
		combos = a.combos
		a.mu.Unlock()
	}
	for _, e := range a.hk.Set(combos) {
		log.Print(e)
	}
}

// PauseHotkeys временно снимает глобальные клавиши (пока в интерфейсе назначают новую).
func (a *App) PauseHotkeys(on bool) {
	a.hkPaused.Store(on)
	a.applyHotkeys()
}

// AttachHotkeys включает глобальные горячие клавиши.
func (a *App) AttachHotkeys() {
	a.hk = winapi.NewHotkeys(func(combo string) {
		a.mu.Lock()
		act := a.actions[combo]
		a.mu.Unlock()
		if act != "" {
			a.Do(act)
		}
	})
}

// Do выполняет действие: sound:<id>[@bus], preset:<name|next|prev>, stop, rosh, mic_monitor, reload, ui.
func (a *App) Do(act string) {
	switch {
	case strings.HasPrefix(act, "sound:"):
		a.Play(strings.TrimPrefix(act, "sound:"))
	case act == "preset:next" || act == "preset:prev":
		a.cyclePreset(act == "preset:next")
	case strings.HasPrefix(act, "preset:"):
		a.SetPreset(strings.TrimPrefix(act, "preset:"), true)
	case act == "stop":
		a.out.Voice().Stop("")
		a.out.Monitor().Stop("")
		log.Print("Все звуки остановлены")
	case act == "rosh":
		a.RoshKilled()
	case act == "mic_monitor":
		on := a.out.ToggleMicMonitor()
		log.Printf("Самопрослушка голоса: %v", map[bool]string{true: "ВКЛ", false: "выкл"}[on])
	case act == "reload":
		if err := a.Reload(); err != nil {
			log.Printf("Ошибка конфига: %v", err)
		}
	case act == "ui":
		if a.OnShowUI != nil {
			a.OnShowUI()
		}
	default:
		log.Printf("Неизвестное действие %q", act)
	}
}

// Play играет звук по ссылке "id" или "id@both|voice|monitor".
func (a *App) Play(ref string) {
	id, busStr, hasBus := strings.Cut(ref, "@")
	a.mu.Lock()
	lib, cfg := a.lib, a.cfg
	a.mu.Unlock()
	s := lib.Get(id)
	if s == nil {
		log.Printf("Звук %q не найден", id)
		return
	}
	if !a.out.Running() {
		log.Printf("Звук %s не сыгран: аудио не запущено (см. «Настройка»)", id)
		return
	}
	clip, ok := s.Pick()
	if !ok {
		return // кулдаун
	}
	bus := s.Bus
	if hasBus {
		bus = sounds.ParseBus(busStr)
	}
	toVoice := bus == sounds.BusBoth || bus == sounds.BusVoice
	toMon := bus == sounds.BusBoth || bus == sounds.BusMonitor

	delay := time.Duration(0)
	if toVoice {
		a.lastVoice.Store(time.Now().UnixNano())
		if a.pressPTT() {
			delay = time.Duration(cfg.PTT.LeadMS) * time.Millisecond
		}
	}
	add := func() {
		if toVoice {
			a.out.Voice().Add(s.ID, clip, s.Volume, s.Mode)
		}
		if toMon {
			a.out.Monitor().Add(s.ID, clip, s.Volume, s.Mode)
		}
	}
	if delay > 0 {
		a.pending.Add(1)
		time.AfterFunc(delay, func() { add(); a.pending.Add(-1) })
	} else {
		add()
	}
	if id != "beep" {
		log.Printf("♪ %s → %s", s.ID, map[sounds.Bus]string{sounds.BusBoth: "войс + вы", sounds.BusVoice: "войс", sounds.BusMonitor: "только вы"}[bus])
	}
}

func (a *App) pressPTT() bool {
	a.mu.Lock()
	vk, auto := a.pttVK, a.cfg.PTT.Auto
	a.mu.Unlock()
	if vk == 0 || !auto || a.pttOwned.Load() {
		return false
	}
	if winapi.IsKeyDown(vk) { // пользователь уже держит кнопку сам — не мешаем
		return false
	}
	if err := winapi.PressKey(vk, true); err != nil {
		log.Printf("PTT: %v", err)
		return false
	}
	a.pttOwned.Store(true)
	return true
}

// PTTLoop отпускает кнопку голосового чата, когда звуки закончились.
func (a *App) PTTLoop() {
	t := time.NewTicker(15 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		if !a.pttOwned.Load() {
			continue
		}
		if a.out.Voice().Active() || a.pending.Load() > 0 {
			a.lastVoice.Store(time.Now().UnixNano())
			continue
		}
		a.mu.Lock()
		tail, vk := time.Duration(a.cfg.PTT.TailMS)*time.Millisecond, a.pttVK
		a.mu.Unlock()
		if time.Since(time.Unix(0, a.lastVoice.Load())) >= tail {
			winapi.PressKey(vk, false)
			a.pttOwned.Store(false)
		}
	}
}

func (a *App) Preset() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.preset
}

func (a *App) SetPreset(name string, announce bool) {
	a.mu.Lock()
	specs, ok := a.presets[name]
	a.mu.Unlock()
	if !ok {
		log.Printf("Пресет %q не найден", name)
		return
	}
	chain, err := dsp.BuildChain(name, specs, sounds.SampleRate)
	if err != nil {
		log.Print(err)
		return
	}
	a.out.SetChain(chain)
	a.mu.Lock()
	a.preset = name
	a.mu.Unlock()
	if announce {
		log.Printf("🎙 Голос: %s", name)
		a.Play("beep@monitor")
	}
}

func (a *App) cyclePreset(forward bool) {
	a.mu.Lock()
	order, cur := a.order, a.preset
	a.mu.Unlock()
	idx := 0
	for i, n := range order {
		if n == cur {
			idx = i
		}
	}
	if forward {
		idx = (idx + 1) % len(order)
	} else {
		idx = (idx - 1 + len(order)) % len(order)
	}
	a.SetPreset(order[idx], true)
}

// PresetNames — порядок пресетов для интерфейса.
func (a *App) PresetNames() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.order...)
}

// Sounds — список звуков для интерфейса.
func (a *App) Sounds() []sounds.Info {
	a.mu.Lock()
	lib := a.lib
	a.mu.Unlock()
	return lib.Infos()
}

func fmtClock(s int) string {
	sign := ""
	if s < 0 {
		sign, s = "-", -s
	}
	return fmt.Sprintf("%s%d:%02d", sign, s/60, s%60)
}

// RoshKilled — отметка убийства Рошана: аегис 5:00, окно респауна 8:00–11:00.
func (a *App) RoshKilled() {
	a.gsiMu.Lock()
	clock, ok := a.timers.Clock()
	if ok {
		a.mu.Lock()
		r := a.cfg.Rosh
		a.mu.Unlock()
		a.timers.AddOneShot("aegis_expire", r.Sound, clock+300, r.AegisWarn)
		a.timers.AddOneShot("rosh_min", r.Sound, clock+480, r.MinWarn)
		a.timers.AddOneShot("rosh_max", r.Sound, clock+660, 0)
	}
	a.gsiMu.Unlock()
	if !ok {
		log.Print("Рошан: нет игрового времени (Dota не подключена через GSI)")
		return
	}
	log.Printf("Рошан убит в %s → аегис до %s, респаун %s–%s", fmtClock(clock), fmtClock(clock+300), fmtClock(clock+480), fmtClock(clock+660))
	a.Play("beep@monitor")
}

// OnGSI обрабатывает пакет состояния от Dota 2.
func (a *App) OnGSI(st *gsi.State) {
	if st.Map == nil {
		a.gsiMu.Lock()
		a.lastGSI = time.Now()
		a.gsiMu.Unlock()
		return
	}
	a.mu.Lock()
	events, lowHP := a.cfg.Events, a.cfg.GSI.LowHP
	a.mu.Unlock()

	a.gsiMu.Lock()
	if a.lastGSI.IsZero() {
		log.Print("GSI: Dota 2 подключена")
	}
	a.lastGSI = time.Now()
	if a.prevState != nil && a.prevState.Map != nil && a.prevState.Map.MatchID != st.Map.MatchID {
		a.timers.Reset()
	}
	var fires []timers.Fire
	if st.Map.GameState == gsi.InProgress || st.Map.GameState == gsi.PreGame {
		fires = a.timers.Update(st.Map.ClockTime)
	}
	evs := gsi.Diff(a.prevState, st, lowHP)
	a.prevState = st
	a.gsiMu.Unlock()

	for _, f := range fires {
		log.Printf("⏱ %s (событие в %s, сейчас %s)", f.TimerID, fmtClock(f.EventAt), fmtClock(st.Map.ClockTime))
		a.Play(f.Sound)
	}
	for _, ev := range evs {
		if ref := events[ev]; ref != "" {
			a.Play(ref)
		}
	}
}

// Status — состояние для интерфейса.
type Status struct {
	Preset     string            `json:"preset"`
	MicMonitor bool              `json:"mic_monitor"`
	Audio      audio.Status      `json:"audio"`
	PTTKey     string            `json:"ptt_key"`
	PTTHeld    bool              `json:"ptt_held"`
	Game       GameStatus        `json:"game"`
	Upcoming   []timers.Upcoming `json:"upcoming"`
}

type GameStatus struct {
	Connected bool   `json:"connected"`
	HasClock  bool   `json:"has_clock"`
	Clock     int    `json:"clock"`
	State     string `json:"state"`
	Hero      string `json:"hero,omitempty"`
	Paused    bool   `json:"paused"`
	Daytime   bool   `json:"daytime"`
}

func (a *App) Status() Status {
	a.mu.Lock()
	s := Status{Preset: a.preset, PTTKey: a.cfg.PTT.Key}
	a.mu.Unlock()
	s.MicMonitor = a.out.MicMonitor()
	s.Audio = a.out.Status()
	s.PTTHeld = a.pttOwned.Load()
	a.gsiMu.Lock()
	s.Game.Connected = !a.lastGSI.IsZero() && time.Since(a.lastGSI) < 40*time.Second
	if st := a.prevState; st != nil && st.Map != nil && s.Game.Connected {
		s.Game.Clock, s.Game.HasClock = st.Map.ClockTime, true
		s.Game.State = strings.TrimPrefix(st.Map.GameState, "DOTA_GAMERULES_STATE_")
		s.Game.Paused, s.Game.Daytime = st.Map.Paused, st.Map.Daytime
		if st.Hero != nil {
			s.Game.Hero = strings.TrimPrefix(st.Hero.Name, "npc_dota_hero_")
		}
		if st.Map.GameState == gsi.InProgress || st.Map.GameState == gsi.PreGame {
			s.Upcoming = a.timers.Upcoming()
		}
	}
	a.gsiMu.Unlock()
	return s
}

// WatchConfig перезагружает конфиг при изменении config.json или папки со звуками.
func (a *App) WatchConfig() {
	a.stamp = a.computeStamp()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for range t.C {
		s := a.computeStamp()
		if s != a.stamp {
			a.stamp = s
			log.Print("Обнаружены изменения — перечитываю конфиг и звуки")
			if err := a.Reload(); err != nil {
				log.Printf("Ошибка конфига: %v", err)
			}
		}
	}
}

func (a *App) computeStamp() string {
	var b strings.Builder
	if st, err := os.Stat(a.ConfigPath); err == nil {
		fmt.Fprintf(&b, "%d;", st.ModTime().UnixNano())
	}
	dir := a.SoundsDir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

// SoundsDir — абсолютный путь к папке звуков.
func (a *App) SoundsDir() string {
	a.mu.Lock()
	dir := "sounds"
	if a.cfg != nil && a.cfg.SoundsDir != "" {
		dir = a.cfg.SoundsDir
	}
	a.mu.Unlock()
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.BaseDir, dir)
	}
	return dir
}
