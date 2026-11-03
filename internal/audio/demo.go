package audio

import (
	"log"
	"math"
	"math/rand"
	"os"

	"github.com/gen2brain/malgo"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Demo — режим без настоящих аудиоустройств (VOICEBOX_DEMO=1): пустой бэкенд miniaudio,
// выдуманный список устройств и «голос» вместо микрофона. Нужен, чтобы разрабатывать
// и проверять интерфейс на машине без звуковой карты.
var Demo = os.Getenv("VOICEBOX_DEMO") != ""

var demoCapture = []Device{
	{Name: "Микрофон (Realtek(R) Audio)", IsDefault: true},
	{Name: "Микрофон (HyperX Cloud II)"},
	{Name: "CABLE Output (VB-Audio Virtual Cable)", Virtual: true},
}

var demoPlayback = []Device{
	{Name: "Динамики (Realtek(R) Audio)", IsDefault: true},
	{Name: "Наушники (HyperX Cloud II)"},
	{Name: "CABLE Input (VB-Audio Virtual Cable)", Virtual: true},
}

func demoDevices() ([]Device, []Device, error) {
	return append([]Device(nil), demoCapture...), append([]Device(nil), demoPlayback...), nil
}

// backendNull — «пустой» бэкенд miniaudio. Константы malgo отстают от miniaudio: после webaudio
// там идёт ma_backend_custom, поэтому malgo.BackendNull (13) на деле означает custom.
const backendNull = malgo.Backend(malgo.BackendNull + 1)

func (e *Engine) startDemoLocked() error {
	ctx, err := malgo.InitContext([]malgo.Backend{backendNull}, malgo.ContextConfig{}, nil)
	if err != nil {
		return err
	}
	e.ctx = ctx
	pick := func(sub string, list []Device, virtual bool) string {
		names := make([]string, len(list))
		for i, d := range list {
			names[i] = d.Name
		}
		if sub != "" {
			if i := matchDevice(names, sub); i >= 0 {
				return names[i]
			}
		}
		for _, d := range list {
			if d.Virtual == virtual && (virtual || d.IsDefault) {
				return d.Name
			}
		}
		return names[0]
	}
	micName := pick(e.opt.MicDevice, demoCapture, false)
	outName := pick(e.opt.VoiceOut, demoPlayback, true)
	monName := pick(e.opt.MonitorOut, demoPlayback, false)

	cfg := malgo.DefaultDeviceConfig(malgo.Duplex)
	cfg.SampleRate = sounds.SampleRate
	cfg.PeriodSizeInMilliseconds = e.opt.PeriodMS
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.Playback.Format = malgo.FormatF32
	cfg.Playback.Channels = 2
	if e.duplex, err = malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: e.onDuplex}); err != nil {
		return err
	}
	mcfg := malgo.DefaultDeviceConfig(malgo.Playback)
	mcfg.SampleRate = sounds.SampleRate
	mcfg.PeriodSizeInMilliseconds = e.opt.PeriodMS
	mcfg.Playback.Format = malgo.FormatF32
	mcfg.Playback.Channels = 2
	if e.monitor, err = malgo.InitDevice(ctx.Context, mcfg, malgo.DeviceCallbacks{Data: e.onMonitor}); err != nil {
		return err
	}
	if err := e.duplex.Start(); err != nil {
		return err
	}
	if err := e.monitor.Start(); err != nil {
		return err
	}
	e.status = Status{Running: true, Mic: micName, VoiceOut: outName, Monitor: monName, CableFound: true}
	e.running.Store(true)
	log.Printf("Аудио (демо): микрофон «%s» → «%s»; наушники «%s»", micName, outName, monName)
	return nil
}

// demoVoice — грубая имитация речи: слоги с основным тоном 110–170 Гц и паузы между фразами.
type demoVoice struct {
	t, left, f0, ph float64
	on              bool
	syl             int
}

func (v *demoVoice) fill(b []float32) {
	const dt = 1.0 / sounds.SampleRate
	for i := range b {
		if v.left <= 0 {
			v.on = !v.on
			switch {
			case v.on:
				v.left = 0.12 + rand.Float64()*0.18
				v.f0 = 110 + rand.Float64()*60
				v.syl++
			case v.syl%7 == 0:
				v.left = 1.2 + rand.Float64()*1.5 // пауза между фразами
			default:
				v.left = 0.04 + rand.Float64()*0.08
			}
			v.t = 0
		}
		v.left -= dt
		v.t += dt
		if !v.on {
			b[i] = float32((rand.Float64()*2 - 1) * 0.002)
			continue
		}
		v.ph += 2 * math.Pi * v.f0 * (1 - 0.1*v.t) * dt
		env := math.Min(1, v.t/0.02) * math.Min(1, v.left/0.03)
		s := 0.0
		for h := 1.0; h <= 12; h++ {
			form := math.Exp(-math.Pow((h*v.f0-700)/400, 2)) + 0.5*math.Exp(-math.Pow((h*v.f0-1500)/500, 2))
			s += math.Sin(h*v.ph) * (0.2 + form) / h
		}
		b[i] = float32(s * env * 0.25)
	}
}
