// Package app связывает всё вместе: горячие клавиши → действия, GSI → события/таймеры,
// автонажатие PTT, пресеты голоса и горячая перезагрузка конфига.
package app

import (
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
}

type App struct {
	ConfigPath string
	BaseDir    string

	mu      sync.Mutex
	cfg     *config.Config
	lib     *sounds.Library
	out     Output
	hk      *winapi.Hotkeys
	timers  *timers.Engine
	presets map[string][]dsp.Spec
	order   []string
	preset  string
	actions map[string]string // текст комбинации → действие
	pttVK   uint32

	prevState *gsi.State
	gsiSeen   bool

	pttOwned  atomic.Bool
	pending   atomic.Int32
	lastVoice atomic.Int64
	stamp     string
	tts       sounds.TTSFunc
}

func New(configPath string, out Output) *App {
	base := filepath.Dir(configPath)
	return &App{
		ConfigPath: configPath, BaseDir: base, out: out,
		timers: timers.New(nil),
		tts:    winapi.NewTTS(filepath.Join(base, "cache"), 1),
	}
}

// Reload (пере)читывает конфиг, звуки, пресеты, таймеры и горячие клавиши.
func (a *App) Reload() error {
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		return err
	}
	a.tts = winapi.NewTTS(filepath.Join(a.BaseDir, "cache"), cfg.TTSRate)
	lib := sounds.Load(a.BaseDir, cfg.SoundsDir, cfg.Sounds, a.tts)

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
		c, err := keys.Parse(cfg.PTT.Key)
		if err != nil {
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
	a.cfg, a.lib, a.presets, a.order, a.actions, a.pttVK = cfg, lib, presets, order, actions, pttVK
	a.timers.SetTimers(cfg.Timers)
	a.mu.Unlock()

	if a.hk != nil {
		for _, e := range a.hk.Set(combos) {
			log.Print(e)
		}
	}
	if first {
		p := cfg.StartPreset
		if p == "" {
			p = "clean"
		}
		a.SetPreset(p, false)
	} else {
		a.SetPreset(a.preset, false) // пересобрать, если пресет изменили в конфиге
	}
	log.Printf("Конфиг загружен: %d звуков, %d горячих клавиш, %d пресетов", len(lib.IDs()), len(combos), len(presets))
	if pttVK == 0 && cfg.PTT.Auto {
		log.Printf("ВНИМАНИЕ: ptt.key не задан — звуки уйдут в войс, только если вы сами зажмёте кнопку голосового чата (или включён открытый микрофон).")
	}
	return nil
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

// Do выполняет действие: sound:<id>[@bus], preset:<name|next|prev>, stop, rosh, mic_monitor, reload.
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
	log.Printf("♪ %s → %s", s.ID, map[sounds.Bus]string{sounds.BusBoth: "войс+вы", sounds.BusVoice: "войс", sounds.BusMonitor: "только вы"}[bus])
}

// pressPTT зажимает кнопку голосового чата, если нужно. true — если нажали сейчас.
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
	for range time.Tick(15 * time.Millisecond) {
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

func fmtClock(s int) string {
	sign := ""
	if s < 0 {
		sign, s = "-", -s
	}
	return fmt.Sprintf("%s%d:%02d", sign, s/60, s%60)
}

// RoshKilled — отметка убийства Рошана: аегис 5:00, окно респауна 8:00–11:00.
func (a *App) RoshKilled() {
	clock, ok := a.timers.Clock()
	if !ok {
		log.Print("Рошан: нет игрового времени (GSI не подключён?)")
		return
	}
	a.mu.Lock()
	r := a.cfg.Rosh
	a.mu.Unlock()
	a.timers.AddOneShot("aegis_expire", r.Sound, clock+300, r.AegisWarn)
	a.timers.AddOneShot("rosh_min", r.Sound, clock+480, r.MinWarn)
	a.timers.AddOneShot("rosh_max", r.Sound, clock+660, 0)
	log.Printf("Рошан убит в %s → аегис до %s, респаун %s–%s", fmtClock(clock), fmtClock(clock+300), fmtClock(clock+480), fmtClock(clock+660))
	a.Play("beep@monitor")
}

// OnGSI обрабатывает пакет состояния от Dota 2.
func (a *App) OnGSI(st *gsi.State) {
	if st.Map == nil {
		return
	}
	if !a.gsiSeen {
		a.gsiSeen = true
		log.Print("GSI: Dota 2 подключена")
	}
	a.mu.Lock()
	events, lowHP := a.cfg.Events, a.cfg.GSI.LowHP
	a.mu.Unlock()

	if st.Map.GameState == gsi.InProgress || st.Map.GameState == gsi.PreGame {
		for _, f := range a.timers.Update(st.Map.ClockTime) {
			log.Printf("⏱ %s (событие в %s, сейчас %s)", f.TimerID, fmtClock(f.EventAt), fmtClock(st.Map.ClockTime))
			a.Play(f.Sound)
		}
	}
	for _, ev := range gsi.Diff(a.prevState, st, lowHP) {
		if ref := events[ev]; ref != "" {
			a.Play(ref)
		}
	}
	if a.prevState != nil && a.prevState.Map != nil && a.prevState.Map.MatchID != st.Map.MatchID {
		a.timers.Reset()
	}
	a.prevState = st
}

// WatchConfig перезагружает конфиг при изменении config.json или папки со звуками.
func (a *App) WatchConfig() {
	a.stamp = a.computeStamp()
	for range time.Tick(2 * time.Second) {
		s := a.computeStamp()
		if s != a.stamp {
			a.stamp = s
			log.Print("Обнаружены изменения — перезагружаю конфиг и звуки")
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
	a.mu.Lock()
	dir := ""
	if a.cfg != nil {
		dir = a.cfg.SoundsDir
	}
	a.mu.Unlock()
	if dir != "" {
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(a.BaseDir, dir)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if info, err := e.Info(); err == nil {
				fmt.Fprintf(&b, "%s:%d:%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	return b.String()
}

// Summary — подсказка при старте.
func (a *App) Summary() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var keysList []string
	for k, v := range a.cfg.Hotkeys {
		keysList = append(keysList, fmt.Sprintf("  %-10s %s", k, v))
	}
	sort.Strings(keysList)
	return "Горячие клавиши:\n" + strings.Join(keysList, "\n")
}
