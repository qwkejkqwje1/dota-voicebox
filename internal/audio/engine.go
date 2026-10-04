package audio

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/gen2brain/malgo"

	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Options — выбор устройств. Громкости меняются на лету через Set*.
type Options struct {
	MicDevice  string // подстрока имени микрофона ("" = по умолчанию)
	VoiceOut   string // имя виртуального кабеля ("" = автопоиск); может быть любое устройство вывода
	CableRec   string // «другой конец» кабеля для тестов ("" = определить автоматически)
	MonitorOut string // наушники ("" = по умолчанию)
	PeriodMS   uint32
}

// afloat — атомарный float32 (для параметров, которые читает аудиопоток).
type afloat struct{ v atomic.Uint32 }

func (a *afloat) Load() float32   { return math.Float32frombits(a.v.Load()) }
func (a *afloat) Store(f float32) { a.v.Store(math.Float32bits(f)) }

// Levels — пиковые уровни для индикаторов в интерфейсе (0..1).
type Levels struct {
	MicIn    float32 `json:"mic_in"`
	VoiceOut float32 `json:"voice_out"`
	Monitor  float32 `json:"monitor"`
}

// Status — что сейчас работает.
type Status struct {
	Running    bool   `json:"running"`
	Error      string `json:"error,omitempty"`
	Mic        string `json:"mic"`
	VoiceOut   string `json:"voice_out"`
	Monitor    string `json:"monitor"`
	CableFound bool   `json:"cable_found"`
}

// Engine: дуплекс (микрофон → кабель) + отдельный вывод в наушники.
type Engine struct {
	mu      sync.Mutex
	opt     Options
	ctx     *malgo.AllocatedContext
	duplex  *malgo.Device
	monitor *malgo.Device
	status  Status

	VoiceMix   Mixer // звуки для команды
	MonitorMix Mixer // звуки для вас

	chain     atomic.Pointer[dsp.Chain]
	micMon    atomic.Bool
	micMonBuf *ring
	fxOnSfx   atomic.Bool
	running   atomic.Bool

	micGain, ducking, sfxVol, monVol afloat
	lvMic, lvOut, lvMon              afloat
	tpOut, tpMon                     afloat
	rawRec                           atomic.Pointer[recorder]

	// дополнительные источники (полосы пульта)
	src            srcManager
	srcList        atomic.Pointer[srcState]
	srcTmp, srcMon []float32
	monTmp         []float32

	// полоса микрофона: mute и шумовой гейт
	micMute  atomic.Bool
	gateThr  afloat // линейный порог, 0 = выкл
	gateGain float32
	gateHold int
	micEnv   float32
	vizPre   tap // микрофон до эффектов
	vizPost  tap // эфир после эффектов

	// буферы аудиопотока (без аллокаций в колбэке после прогрева)
	work, sfx, mono2 []float32
}

func NewEngine() *Engine {
	e := &Engine{micMonBuf: newRing(sounds.SampleRate / 2), gateGain: 1}
	e.src.monRing = newDrift(sounds.SampleRate / 50)
	e.chain.Store(&dsp.Chain{Name: "clean"})
	e.micGain.Store(1)
	e.ducking.Store(0.35)
	e.sfxVol.Store(1)
	e.monVol.Store(0.8)
	return e
}

func (e *Engine) SetChain(c *dsp.Chain) { e.chain.Store(c) }
func (e *Engine) Chain() *dsp.Chain     { return e.chain.Load() }
func (e *Engine) Voice() *Mixer         { return &e.VoiceMix }
func (e *Engine) Monitor() *Mixer       { return &e.MonitorMix }
func (e *Engine) Running() bool         { return e.running.Load() }

// SetMix меняет громкости на лету.
func (e *Engine) SetMix(micGain, ducking, sfxVol, monVol float64, fxOnSounds bool) {
	e.micGain.Store(float32(micGain))
	e.ducking.Store(float32(ducking))
	e.sfxVol.Store(float32(sfxVol))
	e.monVol.Store(float32(monVol))
	e.fxOnSfx.Store(fxOnSounds)
}

func (e *Engine) ToggleMicMonitor() bool {
	v := !e.micMon.Load()
	e.micMon.Store(v)
	e.micMonBuf.Clear()
	return v
}

func (e *Engine) MicMonitor() bool { return e.micMon.Load() }

// Levels возвращает пиковые уровни и сбрасывает их (удержание пика — на стороне UI).
func (e *Engine) Levels() Levels {
	l := Levels{MicIn: e.lvMic.Load(), VoiceOut: e.lvOut.Load(), Monitor: e.lvMon.Load()}
	e.lvMic.Store(0)
	e.lvOut.Store(0)
	e.lvMon.Store(0)
	return l
}

func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

// Device — аудиоустройство для списка в интерфейсе.
type Device struct {
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Virtual   bool   `json:"virtual"` // похоже на виртуальный кабель
}

// Devices возвращает микрофоны и устройства вывода.
func Devices() (capture, playback []Device, err error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, nil, err
	}
	defer func() { ctx.Uninit(); ctx.Free() }()
	conv := func(t malgo.DeviceType) ([]Device, error) {
		ds, err := ctx.Devices(t)
		if err != nil {
			return nil, err
		}
		out := make([]Device, 0, len(ds))
		for i := range ds {
			out = append(out, Device{Name: ds[i].Name(), IsDefault: ds[i].IsDefault != 0, Virtual: IsVirtual(ds[i].Name())})
		}
		return out, nil
	}
	if capture, err = conv(malgo.Capture); err != nil {
		return
	}
	playback, err = conv(malgo.Playback)
	return
}

// ListDevices печатает устройства (для флага -list-devices).
func ListDevices() error {
	c, p, err := Devices()
	if err != nil {
		return err
	}
	pr := func(title string, ds []Device) {
		fmt.Println(title + ":")
		for _, d := range ds {
			def := ""
			if d.IsDefault {
				def = "  [по умолчанию]"
			}
			fmt.Printf("  - %s%s\n", d.Name, def)
		}
	}
	pr("Устройства ввода (микрофоны)", c)
	pr("Устройства вывода", p)
	return nil
}

func findDevice(ctx *malgo.AllocatedContext, t malgo.DeviceType, sub string, skip func(string) bool) (*malgo.DeviceID, string, error) {
	ds, err := ctx.Devices(t)
	if err != nil {
		return nil, "", err
	}
	var names []string
	for i := range ds {
		names = append(names, ds[i].Name())
	}
	if sub == "" {
		// по умолчанию — но не виртуальный кабель
		for i := range ds {
			if ds[i].IsDefault != 0 && (skip == nil || !skip(ds[i].Name())) {
				return nil, ds[i].Name(), nil
			}
		}
		for i := range ds {
			if skip == nil || !skip(ds[i].Name()) {
				id := ds[i].ID
				return &id, ds[i].Name(), nil
			}
		}
		return nil, "по умолчанию", nil
	}
	if i := matchDevice(names, sub); i >= 0 {
		id := ds[i].ID
		return &id, ds[i].Name(), nil
	}
	return nil, "", fmt.Errorf("устройство %q не найдено. Доступны: %s", sub, strings.Join(names, " | "))
}

func f32(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	return unsafe.Slice((*float32)(unsafe.Pointer(&b[0])), len(b)/4)
}

func grow(b []float32, n int) []float32 {
	if cap(b) < n {
		return make([]float32, n, n*2)
	}
	b = b[:n]
	clear(b)
	return b
}

func peak(b []float32, a *afloat) {
	var m float32
	for _, x := range b {
		if x < 0 {
			x = -x
		}
		if x > m {
			m = x
		}
	}
	if m > a.Load() {
		a.Store(m)
	}
}

// Start запускает (или перезапускает) движок с указанными устройствами.
func (e *Engine) Start(opt Options) error {
	e.Close()
	e.mu.Lock()
	defer e.mu.Unlock()
	if opt.PeriodMS == 0 {
		opt.PeriodMS = 10
	}
	e.opt = opt
	err := e.startLocked()
	if err != nil {
		e.status.Error = err.Error()
		e.closeLocked()
	}
	e.mu.Unlock()
	if err == nil {
		e.syncSources()
	}
	e.mu.Lock() // для defer
	return err
}

func (e *Engine) startLocked() error {
	e.status = Status{}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return err
	}
	e.ctx = ctx

	voiceOut := e.opt.VoiceOut
	if voiceOut == "" {
		_, p, _ := Devices()
		voiceOut = FindCable(p)
		if voiceOut == "" {
			return fmt.Errorf("виртуальный кабель не найден — установите VB-Audio Virtual Cable или выберите свой кабель в «Настройка → Устройства»")
		}
	}
	e.status.CableFound = true

	micID, micName, err := findDevice(ctx, malgo.Capture, e.opt.MicDevice, IsCableInput)
	if err != nil {
		return fmt.Errorf("микрофон: %w", err)
	}
	outID, outName, err := findDevice(ctx, malgo.Playback, voiceOut, nil)
	if err != nil {
		e.status.CableFound = false
		return fmt.Errorf("виртуальный кабель: %w", err)
	}
	monID, monName, err := findDevice(ctx, malgo.Playback, e.opt.MonitorOut, func(n string) bool {
		return IsVirtual(n) || n == outName
	})
	if err != nil {
		return fmt.Errorf("наушники: %w", err)
	}

	cfg := malgo.DefaultDeviceConfig(malgo.Duplex)
	cfg.SampleRate = sounds.SampleRate
	cfg.PeriodSizeInMilliseconds = e.opt.PeriodMS
	cfg.PerformanceProfile = malgo.LowLatency
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.Playback.Format = malgo.FormatF32
	cfg.Playback.Channels = 2
	if micID != nil {
		cfg.Capture.DeviceID = micID.Pointer()
	}
	if outID != nil {
		cfg.Playback.DeviceID = outID.Pointer()
	}
	if e.duplex, err = malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: e.onDuplex}); err != nil {
		return fmt.Errorf("микрофон → кабель: %w", err)
	}

	mcfg := malgo.DefaultDeviceConfig(malgo.Playback)
	mcfg.SampleRate = sounds.SampleRate
	mcfg.PeriodSizeInMilliseconds = e.opt.PeriodMS
	mcfg.PerformanceProfile = malgo.LowLatency
	mcfg.Playback.Format = malgo.FormatF32
	mcfg.Playback.Channels = 2
	if monID != nil {
		mcfg.Playback.DeviceID = monID.Pointer()
	}
	if e.monitor, err = malgo.InitDevice(ctx.Context, mcfg, malgo.DeviceCallbacks{Data: e.onMonitor}); err != nil {
		return fmt.Errorf("наушники: %w", err)
	}
	if err := e.duplex.Start(); err != nil {
		return err
	}
	if err := e.monitor.Start(); err != nil {
		return err
	}
	e.status = Status{Running: true, Mic: micName, VoiceOut: outName, Monitor: monName, CableFound: true}
	e.running.Store(true)
	log.Printf("Аудио: микрофон «%s» → «%s»; наушники «%s»", micName, outName, monName)
	return nil
}

func (e *Engine) onDuplex(out, in []byte, frames uint32) {
	n := int(frames)
	o := f32(out)
	e.work = grow(e.work, n)
	e.sfx = grow(e.sfx, n)
	if mic := f32(in); len(mic) >= n {
		if r := e.rawRec.Load(); r != nil {
			r.write(mic[:n])
		}
		g := e.micGain.Load()
		if e.micMute.Load() {
			g = 0
		}
		for i := 0; i < n; i++ {
			e.work[i] = mic[i] * g
		}
		peak(e.work, &e.lvMic)
		e.gate(e.work)
		e.vizPre.write(e.work)
	}
	chain := e.chain.Load()
	playing := e.VoiceMix.Render(e.sfx)
	sv, d := e.sfxVol.Load(), e.ducking.Load()
	if playing && e.fxOnSfx.Load() {
		for i := 0; i < n; i++ {
			e.work[i] = e.work[i]*d + e.sfx[i]*sv
		}
		chain.Process(e.work)
	} else {
		chain.Process(e.work)
		if playing {
			for i := 0; i < n; i++ {
				e.work[i] = e.work[i]*d + e.sfx[i]*sv
			}
		}
	}
	e.mixSources(e.work, e.micEnv)
	softClip(e.work)
	e.vizPost.write(e.work)
	peak(e.work, &e.lvOut)
	peak(e.work, &e.tpOut)
	if e.micMon.Load() {
		e.micMonBuf.Write(e.work)
	}
	for i := 0; i < n && 2*i+1 < len(o); i++ {
		o[2*i], o[2*i+1] = e.work[i], e.work[i]
	}
}

func (e *Engine) onMonitor(out, _ []byte, frames uint32) {
	n := int(frames)
	o := f32(out)
	e.mono2 = grow(e.mono2, n)
	e.MonitorMix.Render(e.mono2)
	if st := e.srcList.Load(); st != nil && len(st.list) > 0 {
		e.monTmp = growNoClear(e.monTmp, n)
		e.src.monRing.Pull(e.monTmp)
		for i := 0; i < n; i++ {
			e.mono2[i] += e.monTmp[i]
		}
	}
	if e.micMon.Load() {
		e.micMonBuf.ReadAdd(e.mono2)
	}
	v := e.monVol.Load()
	for i := 0; i < n; i++ {
		e.mono2[i] *= v
	}
	softClip(e.mono2)
	peak(e.mono2, &e.lvMon)
	peak(e.mono2, &e.tpMon)
	for i := 0; i < n && 2*i+1 < len(o); i++ {
		o[2*i], o[2*i+1] = e.mono2[i], e.mono2[i]
	}
}

func (e *Engine) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closeLocked()
}

func (e *Engine) closeLocked() {
	e.running.Store(false)
	e.closeSources()
	if e.duplex != nil {
		e.duplex.Uninit()
		e.duplex = nil
	}
	if e.monitor != nil {
		e.monitor.Uninit()
		e.monitor = nil
	}
	if e.ctx != nil {
		e.ctx.Uninit()
		e.ctx.Free()
		e.ctx = nil
	}
	e.VoiceMix.Stop("")
	e.MonitorMix.Stop("")
}
