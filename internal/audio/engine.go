package audio

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/gen2brain/malgo"

	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Options — настройки движка.
type Options struct {
	MicDevice     string  // подстрока имени микрофона ("" = по умолчанию)
	VoiceOut      string  // подстрока имени виртуального кабеля ("CABLE Input")
	MonitorOut    string  // наушники ("" = по умолчанию)
	MicGain       float64 // множитель микрофона
	Ducking       float64 // громкость микрофона во время звука (0..1)
	FxOnSounds    bool    // применять пресет голоса и к звукам
	PeriodMS      uint32
	MonitorVolume float64
}

// Engine: дуплекс (микрофон → кабель) + отдельный вывод в наушники.
type Engine struct {
	opt     Options
	ctx     *malgo.AllocatedContext
	duplex  *malgo.Device
	monitor *malgo.Device

	VoiceMix   Mixer // звуки для команды
	MonitorMix Mixer // звуки для вас

	chain     atomic.Pointer[dsp.Chain]
	micMon    atomic.Bool // слышать свой обработанный голос
	micMonBuf *ring

	mu               sync.Mutex
	work, sfx, mono2 []float32
}

func NewEngine(opt Options) *Engine {
	if opt.MicGain == 0 {
		opt.MicGain = 1
	}
	if opt.PeriodMS == 0 {
		opt.PeriodMS = 10
	}
	if opt.MonitorVolume == 0 {
		opt.MonitorVolume = 1
	}
	e := &Engine{opt: opt, micMonBuf: newRing(sounds.SampleRate / 2)}
	e.chain.Store(&dsp.Chain{Name: "clean"})
	return e
}

func (e *Engine) SetChain(c *dsp.Chain) { e.chain.Store(c) }
func (e *Engine) Chain() *dsp.Chain     { return e.chain.Load() }

func (e *Engine) ToggleMicMonitor() bool {
	v := !e.micMon.Load()
	e.micMon.Store(v)
	e.micMonBuf.Clear()
	return v
}

// ListDevices печатает устройства.
func ListDevices() error {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return err
	}
	defer func() { ctx.Uninit(); ctx.Free() }()
	for _, k := range []struct {
		t    malgo.DeviceType
		name string
	}{{malgo.Capture, "Устройства ввода (микрофоны)"}, {malgo.Playback, "Устройства вывода"}} {
		ds, err := ctx.Devices(k.t)
		if err != nil {
			return err
		}
		fmt.Println(k.name + ":")
		for i := range ds {
			def := ""
			if ds[i].IsDefault != 0 {
				def = "  [по умолчанию]"
			}
			fmt.Printf("  - %s%s\n", ds[i].Name(), def)
		}
	}
	return nil
}

func findDevice(ctx *malgo.AllocatedContext, t malgo.DeviceType, sub string) (*malgo.DeviceID, string, error) {
	if sub == "" {
		return nil, "по умолчанию", nil
	}
	ds, err := ctx.Devices(t)
	if err != nil {
		return nil, "", err
	}
	var names []string
	for i := range ds {
		names = append(names, ds[i].Name())
		if strings.Contains(strings.ToLower(ds[i].Name()), strings.ToLower(sub)) {
			id := ds[i].ID
			return &id, ds[i].Name(), nil
		}
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
		return make([]float32, n)
	}
	b = b[:n]
	for i := range b {
		b[i] = 0
	}
	return b
}

func (e *Engine) Start() error {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return err
	}
	e.ctx = ctx

	micID, micName, err := findDevice(ctx, malgo.Capture, e.opt.MicDevice)
	if err != nil {
		return fmt.Errorf("микрофон: %w", err)
	}
	outID, outName, err := findDevice(ctx, malgo.Playback, e.opt.VoiceOut)
	if err != nil {
		return fmt.Errorf("виртуальный кабель: %w", err)
	}
	monID, monName, err := findDevice(ctx, malgo.Playback, e.opt.MonitorOut)
	if err != nil {
		return fmt.Errorf("наушники: %w", err)
	}

	// --- дуплекс: микрофон → эффекты + звуки → кабель ---
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
	e.duplex, err = malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: e.onDuplex})
	if err != nil {
		return fmt.Errorf("дуплекс: %w", err)
	}

	// --- наушники: звуки-напоминания + копия звуков для команды ---
	mcfg := malgo.DefaultDeviceConfig(malgo.Playback)
	mcfg.SampleRate = sounds.SampleRate
	mcfg.PeriodSizeInMilliseconds = e.opt.PeriodMS
	mcfg.PerformanceProfile = malgo.LowLatency
	mcfg.Playback.Format = malgo.FormatF32
	mcfg.Playback.Channels = 2
	if monID != nil {
		mcfg.Playback.DeviceID = monID.Pointer()
	}
	e.monitor, err = malgo.InitDevice(ctx.Context, mcfg, malgo.DeviceCallbacks{Data: e.onMonitor})
	if err != nil {
		return fmt.Errorf("наушники: %w", err)
	}

	if err := e.duplex.Start(); err != nil {
		return err
	}
	if err := e.monitor.Start(); err != nil {
		return err
	}
	log.Printf("Аудио: микрофон «%s» → «%s»; наушники «%s»", micName, outName, monName)
	return nil
}

func (e *Engine) onDuplex(out, in []byte, frames uint32) {
	n := int(frames)
	o := f32(out)
	e.work = grow(e.work, n)
	e.sfx = grow(e.sfx, n)
	if mic := f32(in); len(mic) >= n {
		g := float32(e.opt.MicGain)
		for i := 0; i < n; i++ {
			e.work[i] = mic[i] * g
		}
	}
	chain := e.chain.Load()
	playing := e.VoiceMix.Render(e.sfx)
	if playing && e.opt.FxOnSounds {
		// звук идёт через тот же пресет, что и голос
		for i := 0; i < n; i++ {
			e.work[i] = e.work[i]*float32(e.opt.Ducking) + e.sfx[i]
		}
		chain.Process(e.work)
	} else {
		chain.Process(e.work)
		if playing {
			d := float32(e.opt.Ducking)
			for i := 0; i < n; i++ {
				e.work[i] = e.work[i]*d + e.sfx[i]
			}
		}
	}
	softClip(e.work)
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
	if e.micMon.Load() {
		e.micMonBuf.ReadAdd(e.mono2)
	}
	v := float32(e.opt.MonitorVolume)
	for i := 0; i < n && 2*i+1 < len(o); i++ {
		x := e.mono2[i] * v
		o[2*i], o[2*i+1] = x, x
	}
}

func (e *Engine) Close() {
	if e.duplex != nil {
		e.duplex.Uninit()
	}
	if e.monitor != nil {
		e.monitor.Uninit()
	}
	if e.ctx != nil {
		e.ctx.Uninit()
		e.ctx.Free()
	}
}

func (e *Engine) Voice() *Mixer   { return &e.VoiceMix }
func (e *Engine) Monitor() *Mixer { return &e.MonitorMix }
