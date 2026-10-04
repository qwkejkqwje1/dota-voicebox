package audio

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gen2brain/malgo"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// SourceConfig — дополнительный источник звука (полоса пульта).
//
//	kind "input"    — микрофон / вход аудиоинтерфейса (channel: 0 = все каналы, 1..N — конкретный вход)
//	kind "loopback" — звук, который играет устройство вывода (музыка, браузер, игра)
type SourceConfig struct {
	ID      string  `json:"id"`
	Label   string  `json:"label,omitempty"`
	Kind    string  `json:"kind"`
	Device  string  `json:"device"`
	Channel int     `json:"channel,omitempty"`
	Gain    float64 `json:"gain"`
	Mute    bool    `json:"mute,omitempty"`
	ToAir   float64 `json:"to_air"` // уровень в шину «Эфир» (0..1.5)
	ToMon   float64 `json:"to_mon"` // уровень в шину «Мониторинг»
	Duck    float64 `json:"duck"`   // насколько приглушать, когда вы говорите (0..1)
	Disable bool    `json:"disabled,omitempty"`
}

// StripStatus — состояние полосы для интерфейса.
type StripStatus struct {
	ID      string  `json:"id"`
	Device  string  `json:"device"`
	Running bool    `json:"running"`
	Error   string  `json:"error,omitempty"`
	Level   float32 `json:"level"`
	Ratio   float64 `json:"ratio"` // компенсация дрейфа (1.0 = часы совпадают)
	Xruns   uint64  `json:"xruns"` // опустошения буфера
}

type source struct {
	cfg   SourceConfig
	id    string
	key   string
	dev   *malgo.Device
	drift *drift
	name  string
	err   string

	gain, toAir, toMon, duck afloat
	mute                     atomic.Bool
	lv                       afloat
	conv                     []float32 // буфер колбэка захвата
}

func (c SourceConfig) devKey() string {
	return fmt.Sprintf("%s|%s|%d", c.Kind, strings.ToLower(c.Device), c.Channel)
}

func (s *source) apply(c SourceConfig) {
	s.cfg = c
	s.gain.Store(float32(c.Gain))
	s.toAir.Store(float32(c.ToAir))
	s.toMon.Store(float32(c.ToMon))
	s.duck.Store(float32(c.Duck))
	s.mute.Store(c.Mute)
}

// srcState — набор источников, который видит аудиопоток (меняется атомарно).
type srcState struct{ list []*source }

type srcManager struct {
	mu      sync.Mutex
	want    []SourceConfig
	live    map[string]*source // по ID
	monRing *drift             // смесь источников → наушники
}

// SetSources задаёт дополнительные источники. Громкости и маршруты меняются на лету,
// устройства открываются/закрываются только у изменившихся источников.
func (e *Engine) SetSources(list []SourceConfig) {
	e.src.mu.Lock()
	e.src.want = append([]SourceConfig(nil), list...)
	e.src.mu.Unlock()
	if e.Running() {
		e.syncSources()
	}
}

func (e *Engine) syncSources() {
	m := &e.src
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.live == nil {
		m.live = map[string]*source{}
	}
	keep := map[string]bool{}
	var list []*source
	for _, c := range m.want {
		if c.Disable || c.ID == "" {
			continue
		}
		keep[c.ID] = true
		s := m.live[c.ID]
		if s != nil && (s.key != c.devKey() || s.dev == nil) {
			e.closeSource(s)
			s = nil
		}
		if s == nil {
			s = &source{key: c.devKey(), id: c.ID}
			s.apply(c)
			if err := e.openSource(s); err != nil {
				s.err = err.Error()
				log.Printf("Источник %s: %v", c.ID, err)
			} else {
				log.Printf("Источник %s: «%s»", c.ID, s.name)
			}
			m.live[c.ID] = s
		}
		s.apply(c)
		list = append(list, s)
	}
	for id, s := range m.live {
		if !keep[id] {
			e.closeSource(s)
			delete(m.live, id)
		}
	}
	e.srcList.Store(&srcState{list: list})
}

// RetrySources переоткрывает источники с ошибкой (устройство подключили позже).
func (e *Engine) RetrySources() {
	if !e.Running() {
		return
	}
	e.src.mu.Lock()
	need := false
	for _, s := range e.src.live {
		if s.dev == nil {
			need = true
			delete(e.src.live, s.id)
		}
	}
	e.src.mu.Unlock()
	if need {
		e.syncSources()
	}
}

func (e *Engine) openSource(s *source) error {
	e.mu.Lock()
	ctx := e.ctx
	e.mu.Unlock()
	if ctx == nil {
		return errors.New("аудио не запущено")
	}
	c := s.cfg
	devType := malgo.Capture
	findType := malgo.Capture
	if c.Kind == "loopback" {
		devType, findType = malgo.Loopback, malgo.Playback
	} else if c.Kind != "input" {
		return fmt.Errorf("неизвестный тип источника %q", c.Kind)
	}
	id, name, err := findDevice(ctx, findType, c.Device, nil)
	if err != nil {
		return err
	}
	ch := 1
	if c.Channel > 0 || c.Kind == "loopback" {
		ch = 2
		if c.Channel > 2 {
			ch = c.Channel
		}
	}
	cfg := malgo.DefaultDeviceConfig(devType)
	cfg.SampleRate = sounds.SampleRate
	cfg.PeriodSizeInMilliseconds = 10
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = uint32(ch)
	if id != nil {
		cfg.Capture.DeviceID = id.Pointer()
	}
	s.drift = newDrift(sounds.SampleRate / 50) // цель 20 мс
	pick := c.Channel
	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: func(_, in []byte, frames uint32) {
		x := f32(in)
		n := int(frames)
		if len(s.conv) < n {
			s.conv = make([]float32, n*2) // один раз при прогреве
		}
		out := s.conv[:n]
		switch {
		case ch == 1:
			copy(out, x)
		case pick > 0 && pick <= ch:
			for i := 0; i < n && i*ch+pick-1 < len(x); i++ {
				out[i] = x[i*ch+pick-1]
			}
		default:
			inv := 1 / float32(ch)
			for i := 0; i < n; i++ {
				var sum float32
				for k := 0; k < ch && i*ch+k < len(x); k++ {
					sum += x[i*ch+k]
				}
				out[i] = sum * inv
			}
		}
		s.drift.Write(out)
	}})
	if err != nil {
		return fmt.Errorf("«%s»: %w", name, err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("«%s»: %w", name, err)
	}
	s.dev, s.name, s.err = dev, name, ""
	return nil
}

func (e *Engine) closeSource(s *source) {
	if s.dev != nil {
		s.dev.Uninit()
		s.dev = nil
	}
}

func (e *Engine) closeSources() {
	e.srcList.Store(&srcState{})
	e.src.mu.Lock()
	for id, s := range e.src.live {
		e.closeSource(s)
		delete(e.src.live, id)
	}
	e.src.mu.Unlock()
}

// Strips — состояние дополнительных источников.
func (e *Engine) Strips() []StripStatus {
	e.src.mu.Lock()
	defer e.src.mu.Unlock()
	var out []StripStatus
	for _, c := range e.src.want {
		st := StripStatus{ID: c.ID, Error: "выключен"}
		if s := e.src.live[c.ID]; s != nil {
			st = StripStatus{ID: c.ID, Device: s.name, Running: s.dev != nil, Error: s.err}
			if s.drift != nil {
				st.Ratio, st.Xruns = s.drift.Ratio(), s.drift.Under.Load()
			}
		} else if !c.Disable {
			st.Error = "не запущен"
		}
		out = append(out, st)
	}
	return out
}

// mixSources добавляет источники в эфир (air) и собирает смесь для наушников.
// Вызывается из аудиопотока: без выделений памяти и блокировок.
func (e *Engine) mixSources(air []float32, micEnv float32) {
	st := e.srcList.Load()
	if st == nil || len(st.list) == 0 {
		return
	}
	n := len(air)
	e.srcTmp = growNoClear(e.srcTmp, n)
	e.srcMon = grow(e.srcMon, n)
	anyMon := false
	for _, s := range st.list {
		if s.dev == nil {
			continue
		}
		buf := e.srcTmp[:n]
		s.drift.Pull(buf)
		g := s.gain.Load()
		if d := s.duck.Load(); d > 0 && micEnv > 0.02 {
			g *= 1 - d
		}
		var pk float32
		for i := range buf {
			buf[i] *= g
			if a := abs32(buf[i]); a > pk {
				pk = a
			}
		}
		if pk > s.lv.Load() {
			s.lv.Store(pk)
		}
		if s.mute.Load() {
			continue
		}
		if a := s.toAir.Load(); a > 0 {
			for i := range buf {
				air[i] += buf[i] * a
			}
		}
		if m := s.toMon.Load(); m > 0 {
			anyMon = true
			for i := range buf {
				e.srcMon[i] += buf[i] * m
			}
		}
	}
	if anyMon {
		e.src.monRing.Write(e.srcMon[:n])
	}
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func growNoClear(b []float32, n int) []float32 {
	if cap(b) < n {
		return make([]float32, n, n*2)
	}
	return b[:n]
}

// StripLevels — пиковые уровни источников с прошлого вызова (для индикаторов).
func (e *Engine) StripLevels() map[string]float32 {
	st := e.srcList.Load()
	if st == nil {
		return nil
	}
	out := make(map[string]float32, len(st.list))
	for _, s := range st.list {
		out[s.id] = s.lv.Load()
		s.lv.Store(0)
	}
	return out
}
