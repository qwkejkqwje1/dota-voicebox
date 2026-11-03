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
	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gameclock"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/script"
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

// StripOutput — пульт: полоса микрофона и дополнительные источники (есть у audio.Engine).
type StripOutput interface {
	SetMicStrip(mute bool, gateDB float64)
	SetSources([]audio.SourceConfig)
	Strips() []audio.StripStatus
}

type App struct {
	ConfigPath string
	BaseDir    string
	Version    string

	mu        sync.Mutex
	cfg       *config.Config
	lib       *sounds.Library
	out       Output
	hk        *winapi.Hotkeys
	combos    []keys.Combo
	timers    *timers.Engine
	presets   map[string][]dsp.Spec
	order     []string
	preset    string
	actions   map[string]string
	mouseActs map[[2]uint32]string // сочетания с кнопками мыши (через перехват, RegisterHotKey их не умеет)
	pttVK     uint32
	devOpts   *audio.Options
	speech    Speech
	remLabels map[string]string // id таймера → подпись для «Ближайшие»

	gsiMu     sync.Mutex
	prevState *gsi.State
	rawState  []byte
	lastGSI   time.Time
	seenEv    map[string]bool
	gsiSrv    *gsi.Server
	gsiAddr   string

	clock   *gameclock.Clock
	scripts *script.Manager

	pttOwned  atomic.Bool
	talk      talkCtl
	pending   atomic.Int32
	lastVoice atomic.Int64
	stampMu   sync.Mutex
	stamp     string
	hkPaused  atomic.Bool
	rev       atomic.Int64

	ttsBusy  atomic.Bool
	ttsAgain atomic.Bool
	ttsErr   atomic.Value // string

	remMu    sync.Mutex
	remUntil time.Time // очередь напоминаний: не говорить одновременно

	presetSave *time.Timer
	phrases    []string // недавние фразы из меню поверх игры

	enhMu     sync.Mutex // улучшение голоса: что уже применено
	enhTimbre string
	enhBG     string

	fgMu       sync.Mutex // программа на переднем плане (наборы звуков)
	fgLast     string
	fgRecent   []string
	foreground func() string // для тестов; nil — winapi.ForegroundApp

	// UpdateStatus — состояние автообновления для интерфейса.
	UpdateStatus func() any
	// OnShowUI — вызывается действием "ui" (открыть окно).
	OnShowUI func()
	// OnMenu — действие "menu": показать быстрое меню поверх игры.
	OnMenu func()
	// OnSay — действие "say": быстрое меню сразу в режиме набора текста (поиск или фраза для команды).
	OnSay func()
	// OnWheel — действие "wheel": колесо звуков у курсора. vk — клавиша, которую держат
	// (отпустили — выбор), 0 — открыть без клавиши (выбор щелчком).
	OnWheel func(vk uint32)
	// OnToast — короткое уведомление поверх игры (смена эффекта, напоминание).
	OnToast func(Toast)
	// OnChange — изменились звуки/настройки (интерфейс перечитывает состояние).
	OnChange func()
}

// Toast — всплывающее уведомление.
type Toast struct {
	Emoji string `json:"emoji"`
	Text  string `json:"text"`
	Sub   string `json:"sub,omitempty"`
}

func (a *App) toast(t Toast) {
	if a.OnToast != nil {
		a.OnToast(t)
	}
}

// Notify — уведомление поверх игры (для модулей вне приложения: пульт с телефона).
func (a *App) Notify(t Toast) { a.toast(t) }

// changed — сообщить интерфейсу, что состояние изменилось.
func (a *App) changed() {
	a.rev.Add(1)
	if a.OnChange != nil {
		a.OnChange()
	}
}

// Rev — номер версии состояния (растёт при каждом изменении звуков и настроек).
func (a *App) Rev() int64 { return a.rev.Load() }

func New(configPath string, out Output) *App {
	a := &App{ConfigPath: configPath, BaseDir: filepath.Dir(configPath), out: out, timers: timers.New(nil),
		clock: gameclock.New(nil), seenEv: map[string]bool{}}
	a.scripts = script.NewManager(filepath.Join(a.BaseDir, "scripts"), &scriptHost{a})
	a.scripts.InstallDefaults()
	return a
}

// Scripts — менеджер пользовательских скриптов.
func (a *App) Scripts() *script.Manager { return a.scripts }

// Clock — игровое время.
func (a *App) Clock() *gameclock.Clock { return a.clock }

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
	a.setStamp()
	return err
}

// UpdateConfig — изменить конфиг функцией и сохранить.
func (a *App) UpdateConfig(f func(c *config.Config)) error {
	c := a.Config()
	f(c)
	return a.SaveConfig(c)
}

// saveQuiet сохраняет изменение, которому не нужна полная перезагрузка
// (порядок плиток, ручки эффекта, эффект при запуске): конфиг в памяти обновляется сразу.
func (a *App) saveQuiet(f func(c *config.Config)) error { return a.saveApply(f, nil) }

// saveApply — saveQuiet, а перед оповещением интерфейса — apply (применить изменение к звуку).
func (a *App) saveApply(f func(c *config.Config), apply func(c *config.Config)) error {
	a.mu.Lock()
	b, _ := json.Marshal(a.cfg)
	a.mu.Unlock()
	var c config.Config
	json.Unmarshal(b, &c)
	f(&c)
	c.Fix()
	if err := config.Save(a.ConfigPath, &c); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = &c
	a.mu.Unlock()
	if apply != nil {
		apply(&c)
	}
	a.setStamp()
	a.changed()
	return nil
}

// Reload (пере)читывает конфиг, звуки, пресеты, таймеры, горячие клавиши и устройства.
func (a *App) Reload() error {
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		return err
	}
	a.mu.Lock()
	sp := a.speech
	a.mu.Unlock()
	sp = a.makeSpeech(sp, cfg)
	lib := sounds.Load(a.BaseDir, cfg.SoundsDir, buildDefs(cfg), speechTTS(sp), cfg.Normalize)

	presets, order := buildPresets(cfg)

	var pttVK uint32
	if cfg.PTT.Key != "" {
		if c, err := keys.Parse(cfg.PTT.Key); err != nil {
			log.Printf("ptt.key: %v", err)
		} else {
			pttVK = c.VK
		}
	}

	actions := map[string]string{}
	mouse := map[[2]uint32]string{}
	var combos []keys.Combo
	for k, act := range cfg.Hotkeys {
		c, err := keys.Parse(k)
		if err != nil {
			log.Printf("hotkeys: %v", err)
			continue
		}
		if !actionEnabled(cfg, act) {
			continue
		}
		if isMouseVK(c.VK) {
			mouse[[2]uint32{c.Mods, c.VK}] = act
			continue
		}
		actions[c.Text] = act
		combos = append(combos, c)
	}
	tms, labels := buildTimers(cfg)

	a.mu.Lock()
	first := a.cfg == nil
	a.cfg, a.lib, a.presets, a.order, a.actions, a.pttVK, a.combos = cfg, lib, presets, order, actions, pttVK, combos
	a.mouseActs, a.speech, a.remLabels = mouse, sp, labels
	a.timers.SetTimers(tms)
	a.mu.Unlock()

	a.out.SetMix(cfg.MicGain, cfg.Ducking, or1(cfg.SfxVolume), cfg.MonitorVolume, cfg.FxOnSounds)
	if m, ok := a.out.(StripOutput); ok {
		m.SetMicStrip(cfg.Mic.Mute, cfg.Mic.GateDB)
		m.SetSources(cfg.Sources)
	}
	a.applyEnhance(cfg, lib)
	a.applyTalk(cfg, pttVK)
	a.applyHotkeys()
	a.scripts.Sync(cfg.Scripts)
	a.applyGSI(cfg)

	// устройства: перезапуск движка только если выбор изменился
	opts := audio.Options{MicDevice: cfg.Devices.Mic, VoiceOut: cfg.Devices.VoiceOut, CableRec: cfg.Devices.CableRec, MonitorOut: cfg.Devices.Monitor}
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
		if _, ok := presets[p]; !ok {
			p = "clean"
		}
		a.SetPreset(p, false)
	} else {
		a.SetPreset(a.Preset(), false)
	}
	log.Printf("Конфиг применён: %d звуков, %d горячих клавиш, %d эффектов", len(lib.IDs()), len(combos)+len(mouse), len(presets))
	a.renderSpeech(lib)
	a.changed()
	return nil
}

// buildPresets — встроенные эффекты (с ручками из preset_params) и свои из voice_presets.
func buildPresets(cfg *config.Config) (map[string][]dsp.Spec, []string) {
	presets := map[string][]dsp.Spec{}
	for _, p := range dsp.Catalog {
		presets[p.ID] = p.Specs(cfg.PresetParams[p.ID])
	}
	var custom []string
	for k, v := range cfg.VoicePresets {
		if _, err := dsp.BuildChain(k, v, sounds.SampleRate); err != nil {
			log.Printf("пресет %s пропущен: %v", k, err)
			continue
		}
		if _, builtin := presets[k]; !builtin {
			custom = append(custom, k)
		}
		presets[k] = v
	}
	sort.Strings(custom)
	return presets, append(append([]string{}, dsp.PresetOrder...), custom...)
}

// actionEnabled — действия модуля Dota не работают, пока модуль выключен.
func actionEnabled(cfg *config.Config, act string) bool {
	switch act {
	case "menu":
		return cfg.QuickMenu.Enabled
	case "wheel":
		return cfg.Wheel.Enabled
	}
	if cfg.Dota.Enabled {
		return true
	}
	if id, ok := strings.CutPrefix(act, "sound:"); ok {
		id, _, _ = strings.Cut(id, "@")
		return !cfg.Sounds[id].Dota
	}
	return act != "rosh" && !strings.HasPrefix(act, "clock_")
}

func isMouseVK(vk uint32) bool { return vk >= 0x04 && vk <= 0x06 }

func (a *App) logf(f string, args ...any) { log.Printf(f, args...) }

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
	a.scripts.SetPaused(on)
	a.applyHotkeys()
}

// KeyEvent — нажатие из глобального перехвата клавиатуры и мыши (скрипты, режим рации, кнопки мыши).
func (a *App) KeyEvent(ev combo.Event) {
	a.talkKey(ev)
	a.scripts.Key(ev)
	if ev.Down && isMouseVK(ev.VK) && !a.hkPaused.Load() {
		var mods uint32
		if winapi.IsKeyDown(0x11) {
			mods |= keys.ModCtrl
		}
		if winapi.IsKeyDown(0x12) {
			mods |= keys.ModAlt
		}
		if winapi.IsKeyDown(0x10) {
			mods |= keys.ModShift
		}
		if winapi.IsKeyDown(0x5B) || winapi.IsKeyDown(0x5C) {
			mods |= keys.ModWin
		}
		a.mu.Lock()
		act := a.mouseActs[[2]uint32{mods, ev.VK}]
		a.mu.Unlock()
		if act != "" {
			go a.doKey(act, ev.VK)
		}
	}
}

// AttachHotkeys включает глобальные горячие клавиши.
func (a *App) AttachHotkeys() {
	a.hk = winapi.NewHotkeys(func(combo string) {
		a.mu.Lock()
		act := a.actions[combo]
		a.mu.Unlock()
		if act != "" {
			var vk uint32
			if c, err := keys.Parse(combo); err == nil {
				vk = c.VK
			}
			a.doKey(act, vk)
		}
	})
}

// doKey — действие по горячей клавише vk (колесу нужна клавиша: его держат и отпускают).
func (a *App) doKey(act string, vk uint32) {
	if act == "wheel" {
		a.openWheel(vk)
		return
	}
	a.Do(act)
}

func (a *App) openWheel(vk uint32) {
	if a.OnWheel != nil {
		a.OnWheel(vk)
	} else {
		log.Print("Колесо звуков доступно только в Windows")
	}
}

// Do выполняет действие: sound:<id>[@bus], preset:<name|next|prev>, set:<id|next|prev|all>, stop, menu, wheel, say,
// rosh, mic_monitor, reload, ui, talk, mic_mute, denoise, bg.
func (a *App) Do(act string) {
	switch {
	case strings.HasPrefix(act, "sound:"):
		a.Play(strings.TrimPrefix(act, "sound:"))
	case act == "preset:next" || act == "preset:prev":
		a.cyclePreset(act == "preset:next")
	case strings.HasPrefix(act, "preset:"):
		a.SetPreset(strings.TrimPrefix(act, "preset:"), true)
	case act == "stop":
		a.StopAll()
	case act == "menu":
		if a.OnMenu != nil {
			a.OnMenu()
		} else {
			log.Print("Быстрое меню доступно только в Windows")
		}
	case act == "wheel":
		a.openWheel(0)
	case act == "say":
		if a.OnSay != nil {
			a.OnSay()
		} else {
			log.Print("Набор текста поверх игры доступен только в Windows")
		}
	case act == "denoise":
		a.toggleDenoise()
	case act == "bg":
		a.toggleBackground()
	case act == "set:next" || act == "set:prev":
		a.cycleSet(act == "set:next")
	case act == "set:all":
		a.SetActiveSet("", true)
	case strings.HasPrefix(act, "set:"):
		if err := a.SetActiveSet(strings.TrimPrefix(act, "set:"), true); err != nil {
			log.Print(err)
		}
	case act == "rosh":
		a.RoshKilled()
	case act == "mic_monitor":
		on := a.out.ToggleMicMonitor()
		log.Printf("Самопрослушка голоса: %v", map[bool]string{true: "ВКЛ", false: "выкл"}[on])
	case act == "reload":
		if err := a.Reload(); err != nil {
			log.Printf("Ошибка конфига: %v", err)
		}
	case act == "talk":
		a.TalkToggle()
	case act == "mic_mute":
		a.UpdateConfig(func(c *config.Config) { c.Mic.Mute = !c.Mic.Mute })
		log.Printf("Микрофон: %s", map[bool]string{true: "ВЫКЛЮЧЕН (mute)", false: "включён"}[a.Config().Mic.Mute])
	case strings.HasPrefix(act, "source_mute:"):
		id := strings.TrimPrefix(act, "source_mute:")
		a.UpdateConfig(func(c *config.Config) {
			for i := range c.Sources {
				if c.Sources[i].ID == id {
					c.Sources[i].Mute = !c.Sources[i].Mute
				}
			}
		})
	case act == "clock_horn":
		a.SetManualClock(0)
	case strings.HasPrefix(act, "clock_sync:"):
		sec, err := script.ParseTime(strings.TrimPrefix(act, "clock_sync:"))
		if err != nil {
			log.Printf("clock_sync: %v", err)
			return
		}
		a.SetManualClock(sec)
	case act == "clock_clear":
		a.clock.ClearManual()
		log.Print("Ручное время сброшено")
	case act == "ui":
		if a.OnShowUI != nil {
			a.OnShowUI()
		}
	default:
		log.Printf("Неизвестное действие %q", act)
	}
}

// Play играет звук по ссылке "id" или "id@both|voice|monitor".
func (a *App) Play(ref string) { a.PlayOpts(ref, 1) }

// StopAll плавно останавливает все звуки.
func (a *App) StopAll() {
	a.out.Voice().Stop("")
	a.out.Monitor().Stop("")
	log.Print("Все звуки остановлены")
}

// StopSound останавливает один звук.
func (a *App) StopSound(id string) {
	a.out.Voice().Stop(id)
	a.out.Monitor().Stop(id)
}

// IsPlaying — играет ли звук сейчас.
func (a *App) IsPlaying(id string) bool { return a.out.Voice().Has(id) || a.out.Monitor().Has(id) }

// Playing — что играет (для полосок прогресса).
func (a *App) Playing() []audio.Playing {
	return audio.MergePlaying(a.out.Voice().Playing(), a.out.Monitor().Playing())
}

// PlayOpts — Play с множителем громкости.
func (a *App) PlayOpts(ref string, volume float64) error {
	id, busStr, hasBus := strings.Cut(ref, "@")
	a.mu.Lock()
	lib := a.lib
	a.mu.Unlock()
	s := lib.Get(id)
	if s == nil {
		log.Printf("Звук %q не найден", id)
		return fmt.Errorf("звук %q не найден", id)
	}
	if s.Mode == sounds.ModeToggle && a.IsPlaying(s.ID) {
		a.StopSound(s.ID) // повторное нажатие — стоп
		return nil
	}
	bus := s.Bus
	if hasBus {
		bus = sounds.ParseBus(busStr)
	}
	clip, ok := s.Pick()
	if !ok {
		if len(s.Clips) == 0 {
			if s.Pending {
				return fmt.Errorf("звук %q ещё готовится (синтез речи)", id)
			}
			return fmt.Errorf("звук %q не загрузился: %s", id, s.Err)
		}
		return nil // кулдаун
	}
	return a.playClip(s.ID, clip, float32(float64(s.Volume)*volume), s.Mode, bus)
}

func (a *App) playClip(id string, clip sounds.Clip, vol float32, mode string, bus sounds.Bus) error {
	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	if !a.out.Running() {
		log.Printf("Звук %s не сыгран: аудио не запущено (см. страницу «Проверка»)", id)
		return fmt.Errorf("аудио не запущено")
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
			a.out.Voice().Add(id, clip, vol, mode)
		}
		if toMon {
			a.out.Monitor().Add(id, clip, vol, mode)
		}
	}
	if delay > 0 {
		a.pending.Add(1)
		time.AfterFunc(delay, func() { add(); a.pending.Add(-1) })
	} else {
		add()
	}
	if id != "beep" && id != "preview" {
		log.Printf("♪ %s → %s", id, map[sounds.Bus]string{sounds.BusBoth: "команда и вы", sounds.BusVoice: "команда", sounds.BusMonitor: "только вы"}[bus])
	}
	return nil
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

// SetPreset включает эффект голоса. announce — выбор пользователя (клавиша, меню, интерфейс):
// показать уведомление и запомнить эффект до следующего запуска.
func (a *App) SetPreset(name string, announce bool) {
	a.mu.Lock()
	specs, ok := a.presets[name]
	prev := a.preset
	a.mu.Unlock()
	if !ok {
		log.Printf("Эффект %q не найден", name)
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
	if !announce {
		return
	}
	title, emoji := PresetTitle(name)
	log.Printf("🎙 Голос: %s", title)
	a.toast(Toast{Emoji: emoji, Text: title, Sub: "эффект голоса"})
	if a.OnToast == nil {
		a.PlayOpts("beep@monitor", 0.5)
	}
	if prev != name {
		a.rememberPreset(name)
		a.changed()
	}
}

// rememberPreset сохраняет эффект «при запуске» (с задержкой — при быстром перелистывании пишем один раз).
func (a *App) rememberPreset(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.presetSave != nil {
		a.presetSave.Stop()
	}
	a.presetSave = time.AfterFunc(1500*time.Millisecond, func() {
		if a.Config().StartPreset != name {
			a.saveQuiet(func(c *config.Config) { c.StartPreset = name })
		}
	})
}

// PresetTitle — название и значок эффекта для уведомлений.
func PresetTitle(id string) (title, emoji string) {
	if p, ok := dsp.FindPreset(id); ok {
		return p.Title, p.Emoji
	}
	return id, "🎛️"
}

// SetPresetParams меняет ручки эффекта и сразу применяет, если эффект включён.
func (a *App) SetPresetParams(id string, vals map[string]float64) error {
	p, ok := dsp.FindPreset(id)
	if !ok {
		return fmt.Errorf("эффект %q не найден", id)
	}
	vals = p.Values(vals)
	if err := a.saveQuiet(func(c *config.Config) {
		if c.PresetParams == nil {
			c.PresetParams = map[string]map[string]float64{}
		}
		c.PresetParams[id] = vals
	}); err != nil {
		return err
	}
	a.mu.Lock()
	_, custom := a.cfg.VoicePresets[id]
	if !custom {
		a.presets[id] = p.Specs(vals)
	}
	cur := a.preset
	a.mu.Unlock()
	if cur == id && !custom {
		a.SetPreset(id, false)
	}
	return nil
}

// ResetPresetParams возвращает ручки эффекта к значениям по умолчанию.
func (a *App) ResetPresetParams(id string) error {
	p, ok := dsp.FindPreset(id)
	if !ok {
		return fmt.Errorf("эффект %q не найден", id)
	}
	return a.SetPresetParams(id, p.Values(nil))
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
	snap := a.clock.Now()
	clock, ok := snap.Clock, snap.OK
	a.mu.Lock()
	r, enabled := a.cfg.Rosh, a.cfg.Dota.Enabled && a.cfg.Dota.Rosh
	a.mu.Unlock()
	if !enabled {
		return
	}
	a.gsiMu.Lock()
	if ok {
		a.timers.AddOneShot("aegis_expire", roshSound("aegis"), clock+300, r.AegisWarn)
		a.timers.AddOneShot("rosh_min", roshSound("min"), clock+480, r.MinWarn)
		a.timers.AddOneShot("rosh_max", roshSound("max"), clock+660, 0)
	}
	a.gsiMu.Unlock()
	if !ok {
		log.Print("Рошан: нет игрового времени (Dota не подключена через GSI)")
		return
	}
	log.Printf("Рошан убит в %s → аегис до %s, респаун %s–%s", fmtClock(clock), fmtClock(clock+300), fmtClock(clock+480), fmtClock(clock+660))
	a.toast(Toast{Emoji: "🐲", Text: "Рошан убит", Sub: fmt.Sprintf("респаун %s–%s", fmtClock(clock+480), fmtClock(clock+660))})
	a.PlayOpts("beep@monitor", 0.6)
}

// OnGSI обрабатывает пакет состояния от Dota 2.
func (a *App) OnGSI(st *gsi.State, raw []byte) {
	if st.Map == nil {
		a.gsiMu.Lock()
		a.lastGSI = time.Now()
		a.gsiMu.Unlock()
		return
	}
	a.mu.Lock()
	events, lowHP := a.cfg.Events, a.cfg.GSI.LowHP
	a.mu.Unlock()

	running := st.Map.GameState == gsi.InProgress || st.Map.GameState == gsi.PreGame
	newGame := a.clock.OnGSI(st.Map.ClockTime, running, st.Map.Paused, st.Map.MatchID)

	a.gsiMu.Lock()
	if a.lastGSI.IsZero() {
		log.Print("GSI: Dota 2 подключена")
	}
	a.lastGSI = time.Now()
	if newGame || (a.prevState != nil && a.prevState.Map != nil && a.prevState.Map.MatchID != st.Map.MatchID) {
		a.timers.Reset()
		a.seenEv = map[string]bool{}
		newGame = true
	}
	evs := gsi.Diff(a.prevState, st, lowHP)
	// события провайдера "events": Рошан убит и т.п. (приходят повторно — дедуп по типу+времени)
	var roshKilled bool
	for _, e := range st.Events {
		key := fmt.Sprintf("%s@%d", e.EventType, e.GameTime)
		if a.seenEv[key] {
			continue
		}
		a.seenEv[key] = true
		if a.prevState == nil { // старые события из первого пакета не повторяем
			continue
		}
		evs = append(evs, e.EventType)
		if e.EventType == "roshan_killed" {
			roshKilled = true
		}
	}
	a.prevState = st
	a.rawState = raw
	a.gsiMu.Unlock()

	if newGame {
		a.scripts.Event("new_game", nil)
	}
	a.tickClock()
	if roshKilled {
		log.Print("GSI: Рошан убит — запускаю таймеры автоматически")
		a.RoshKilled()
		evs = append(evs, "rosh_killed")
	}
	for _, ev := range evs {
		if ref := events[ev]; ref != "" {
			a.Play(ref)
		}
		a.scripts.Event(ev, nil)
	}
	a.scripts.Event("state", nil)
}

// SetManualClock — задать игровое время вручную (если GSI нет): 0 = горн сейчас.
func (a *App) SetManualClock(sec int) {
	a.gsiMu.Lock()
	a.timers.Reset()
	a.gsiMu.Unlock()
	a.clock.SetManual(float64(sec))
	a.scripts.Event("new_game", nil)
	log.Printf("⏱ Игровое время задано вручную: %s (используется, пока нет данных от Dota)", fmtClock(sec))
}

// ClockLoop — двигает таймеры и скрипты по сглаженному игровому времени.
func (a *App) ClockLoop() {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for range t.C {
		a.tickClock()
	}
}

func (a *App) tickClock() {
	snap := a.clock.Now()
	if !snap.OK {
		return
	}
	a.gsiMu.Lock()
	fires := a.timers.Update(snap.Clock)
	a.gsiMu.Unlock()
	for _, f := range fires {
		log.Printf("⏱ %s (событие в %s, сейчас %s)", f.TimerID, fmtClock(f.EventAt), fmtClock(snap.Clock))
		a.playReminder(f.Sound)
	}
	a.scripts.Clock(snap)
}

// Status — состояние для интерфейса.
type Status struct {
	Rev        int64               `json:"rev"`
	Preset     string              `json:"preset"`
	MicMonitor bool                `json:"mic_monitor"`
	Audio      audio.Status        `json:"audio"`
	PTTKey     string              `json:"ptt_key"`
	PTTHeld    bool                `json:"ptt_held"`
	Game       GameStatus          `json:"game"`
	Upcoming   []timers.Upcoming   `json:"upcoming"`
	Strips     []audio.StripStatus `json:"strips"`
	MicMute    bool                `json:"mic_mute"`
	Talk       TalkStatus          `json:"talk"`
	Dota       bool                `json:"dota"`
	TTSBusy    bool                `json:"tts_busy"`
	TTSError   string              `json:"tts_error,omitempty"`
	Update     any                 `json:"update,omitempty"`
	Denoise    bool                `json:"denoise"`              // шумоподавление работает
	Set        string              `json:"set,omitempty"`        // включённый набор звуков
	Foreground string              `json:"foreground,omitempty"` // программа на экране
}

type GameStatus struct {
	ClockInfo gameclock.Snapshot `json:"clock_info"`
	Connected bool               `json:"connected"`
	HasClock  bool               `json:"has_clock"`
	Clock     int                `json:"clock"`
	State     string             `json:"state"`
	Hero      string             `json:"hero,omitempty"`
	Paused    bool               `json:"paused"`
	Daytime   bool               `json:"daytime"`
}

func (a *App) Status() Status {
	a.mu.Lock()
	s := Status{Rev: a.rev.Load(), Preset: a.preset, PTTKey: a.cfg.PTT.Key, MicMute: a.cfg.Mic.Mute, Dota: a.cfg.Dota.Enabled,
		Set: a.cfg.ActiveSet}
	labels := a.remLabels
	a.mu.Unlock()
	if e, ok := a.out.(EnhanceOutput); ok {
		s.Denoise = e.DenoiseOn()
	}
	a.fgMu.Lock()
	s.Foreground = a.fgLast
	a.fgMu.Unlock()
	s.TTSBusy = a.ttsBusy.Load()
	if e, _ := a.ttsErr.Load().(string); e != "" {
		s.TTSError = e
	}
	s.Talk = a.talkStatus()
	s.MicMonitor = a.out.MicMonitor()
	if m, ok := a.out.(StripOutput); ok {
		s.Strips = m.Strips()
	}
	if a.UpdateStatus != nil {
		s.Update = a.UpdateStatus()
	}
	s.Audio = a.out.Status()
	s.PTTHeld = a.pttOwned.Load()
	snap := a.clock.Now()
	s.Game.ClockInfo = snap
	a.gsiMu.Lock()
	s.Game.Connected = !a.lastGSI.IsZero() && time.Since(a.lastGSI) < 40*time.Second
	if st := a.prevState; st != nil && st.Map != nil && s.Game.Connected {
		s.Game.State = strings.TrimPrefix(st.Map.GameState, "DOTA_GAMERULES_STATE_")
		s.Game.Paused, s.Game.Daytime = st.Map.Paused, st.Map.Daytime
		if st.Hero != nil {
			s.Game.Hero = strings.TrimPrefix(st.Hero.Name, "npc_dota_hero_")
		}
	}
	if snap.OK {
		s.Game.Clock, s.Game.HasClock = snap.Clock, true
		s.Game.Paused = snap.Paused
		s.Upcoming = a.timers.Upcoming()
	}
	a.gsiMu.Unlock()
	for i := range s.Upcoming {
		if l := labels[s.Upcoming[i].ID]; l != "" {
			s.Upcoming[i].Label = l
		}
	}
	return s
}

// WatchConfig перезагружает конфиг при изменении config.json или папки со звуками.
func (a *App) WatchConfig() {
	a.setStamp()
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for range t.C {
		s := a.computeStamp()
		a.stampMu.Lock()
		same := s == a.stamp
		a.stamp = s
		a.stampMu.Unlock()
		if !same {
			log.Print("Обнаружены изменения — перечитываю конфиг и звуки")
			if err := a.Reload(); err != nil {
				log.Printf("Ошибка конфига: %v", err)
			}
		}
	}
}

// setStamp запоминает состояние файлов после собственной записи — чтобы не перечитывать их зря.
func (a *App) setStamp() {
	s := a.computeStamp()
	a.stampMu.Lock()
	a.stamp = s
	a.stampMu.Unlock()
}

func (a *App) computeStamp() string {
	var b strings.Builder
	if st, err := os.Stat(a.ConfigPath); err == nil {
		fmt.Fprintf(&b, "%d;", st.ModTime().UnixNano())
	}
	for _, dir := range []string{a.SoundsDir(), a.scripts.Dir} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if info, err := e.Info(); err == nil {
				fmt.Fprintf(&b, "%s:%d:%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
			}
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
