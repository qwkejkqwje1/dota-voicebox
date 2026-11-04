package app

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// Speech — синтез речи: Windows (голоса системы) или заглушка в демо-режиме.
type Speech interface {
	Cached(text string) (sounds.Clip, bool)
	Failed(text string) error
	Render(texts []string) error
	Say(text string) (sounds.Clip, error)
	// SayWith — фраза другим голосом, скоростью или языком («Читалка»).
	SayWith(text string, o winapi.SpeakOpts) (sounds.Clip, error)
	Voices() ([]winapi.Voice, error)
	Retry()
}

// makeSpeech переиспользует синтезатор, если голос и скорость не изменились.
func (a *App) makeSpeech(cur Speech, cfg *config.Config) Speech {
	dir := filepath.Join(a.BaseDir, "cache")
	lang := "" // голос фраз по умолчанию — русский; в английском интерфейсе — английский
	if i18n.En() {
		lang = "en"
	}
	if t, ok := cur.(*winapi.TTS); ok && t != nil && t.Voice == cfg.TTSVoice && t.Rate == cfg.TTSRate && t.Lang == lang {
		return cur
	}
	if f, ok := cur.(*fakeSpeech); ok && f.rate == cfg.TTSRate && f.voice == cfg.TTSVoice {
		return cur
	}
	if runtime.GOOS != "windows" {
		if audio.Demo {
			return &fakeSpeech{mem: map[string]sounds.Clip{}, rate: cfg.TTSRate, voice: cfg.TTSVoice}
		}
		return nil
	}
	if t := winapi.NewTTS(dir, cfg.TTSVoice, cfg.TTSRate); t != nil {
		t.Lang = lang
		return t
	}
	return nil
}

// speechTTS — интерфейс для библиотеки звуков (nil, если синтеза нет).
func speechTTS(sp Speech) sounds.TTS {
	if sp == nil {
		return nil
	}
	return sp
}

// renderSpeech синтезирует недостающие фразы в фоне и перечитывает библиотеку.
// Одновременно работает только один синтез; новые запросы подхватываются после.
func (a *App) renderSpeech(lib *sounds.Library) {
	a.mu.Lock()
	sp := a.speech
	a.mu.Unlock()
	if sp == nil || len(lib.Pending()) == 0 {
		return
	}
	if !a.ttsBusy.CompareAndSwap(false, true) {
		a.ttsAgain.Store(true)
		return
	}
	a.changed()
	go func() {
		defer func() {
			a.ttsBusy.Store(false)
			a.changed()
		}()
		pend := lib.Pending()
		for len(pend) > 0 {
			t0 := time.Now()
			if err := sp.Render(pend); err != nil {
				log.Printf("Синтез речи: %v", err)
				a.ttsErr.Store(err.Error())
			} else {
				a.ttsErr.Store("")
				log.Printf("Синтез речи: готово %d фраз за %.1f с", len(pend), time.Since(t0).Seconds())
			}
			a.ttsAgain.Store(false)
			lib = a.reloadLibrary()
			pend = lib.Pending()
		}
	}()
}

// reloadLibrary пересобирает только библиотеку звуков (без устройств и клавиш).
func (a *App) reloadLibrary() *sounds.Library {
	a.mu.Lock()
	cfg, sp := a.cfg, a.speech
	a.mu.Unlock()
	lib := sounds.Load(a.BaseDir, cfg.SoundsDir, buildDefs(cfg), speechTTS(sp), cfg.Normalize)
	a.mu.Lock()
	if a.cfg == cfg {
		a.lib = lib
	}
	a.mu.Unlock()
	a.changed()
	return lib
}

// RetrySpeech — повторить синтез фраз, которые не получились.
func (a *App) RetrySpeech() {
	a.mu.Lock()
	sp := a.speech
	a.mu.Unlock()
	if sp == nil {
		return
	}
	sp.Retry()
	a.ttsErr.Store("")
	a.renderSpeech(a.reloadLibrary())
}

// Voices — голоса синтеза речи.
func (a *App) Voices() ([]winapi.Voice, error) {
	a.mu.Lock()
	sp := a.speech
	a.mu.Unlock()
	if sp == nil {
		return nil, fmt.Errorf("синтез речи доступен только в Windows")
	}
	return sp.Voices()
}

// Say произносит фразу (синтез при необходимости) — для скриптов и кнопки «Сказать».
func (a *App) Say(text string, bus sounds.Bus) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("пустая фраза")
	}
	a.mu.Lock()
	sp := a.speech
	norm := a.cfg.Normalize
	a.mu.Unlock()
	if sp == nil {
		return fmt.Errorf("синтез речи доступен только в Windows")
	}
	clip, err := sp.Say(text)
	if err != nil {
		return err
	}
	if norm {
		clip = sounds.Normalize(clip)
	}
	return a.playClip("say", clip, 1, sounds.ModeInterrupt, bus)
}

// fakeSpeech — «синтез» для демо-режима вне Windows: на каждое слово короткий слог.
type fakeSpeech struct {
	mu    sync.Mutex
	mem   map[string]sounds.Clip
	rate  int
	voice string
}

func (f *fakeSpeech) Cached(text string) (sounds.Clip, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.mem[text]
	return c, ok
}

func (f *fakeSpeech) Failed(string) error { return nil }
func (f *fakeSpeech) Retry()              {}

func (f *fakeSpeech) Render(texts []string) error {
	time.Sleep(600 * time.Millisecond)
	for _, t := range texts {
		c := fakeVoice(t)
		f.mu.Lock()
		f.mem[t] = c
		f.mu.Unlock()
	}
	return nil
}

func (f *fakeSpeech) Say(text string) (sounds.Clip, error) {
	if c, ok := f.Cached(text); ok {
		return c, nil
	}
	f.Render([]string{text})
	c, _ := f.Cached(text)
	return c, nil
}

// SayWith — мужской голос ниже, скорость меняет длину слогов.
func (f *fakeSpeech) SayWith(text string, o winapi.SpeakOpts) (sounds.Clip, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("пустая фраза")
	}
	time.Sleep(150 * time.Millisecond)
	f0 := 150.0
	if o.Gender == "Male" || strings.Contains(o.Voice, "Pavel") || strings.Contains(o.Voice, "David") {
		f0 = 105
	}
	return fakeVoiceAt(text, f0, math.Pow(0.9, float64(o.Rate))), nil
}

func (f *fakeSpeech) Voices() ([]winapi.Voice, error) {
	return []winapi.Voice{
		{ID: "winrt:Microsoft Irina", Name: "Microsoft Irina", Lang: "ru-RU", Gender: "Female"},
		{ID: "winrt:Microsoft Pavel", Name: "Microsoft Pavel", Lang: "ru-RU", Gender: "Male"},
		{ID: "winrt:Microsoft Svetlana", Name: "Microsoft Svetlana", Lang: "ru-RU", Gender: "Female"},
		{ID: "winrt:Microsoft David", Name: "Microsoft David", Lang: "en-US", Gender: "Male"},
	}, nil
}

func fakeVoice(text string) sounds.Clip { return fakeVoiceAt(text, 150, 1) }

// fakeVoiceAt — слоги с основным тоном base…base+40 Гц; slow > 1 — медленнее.
func fakeVoiceAt(text string, base, slow float64) sounds.Clip {
	const sr = sounds.SampleRate
	var out sounds.Clip
	for _, w := range strings.Fields(text) {
		n := len([]rune(w))
		d := (0.09 + 0.045*float64(n)) * slow
		f0 := base + rand.Float64()*40
		seg := make(sounds.Clip, int(d*sr))
		ph := 0.0
		for i := range seg {
			t := float64(i) / sr
			ph += 2 * math.Pi * f0 * (1 - 0.15*t/d) / sr
			env := math.Min(1, t/0.015) * math.Min(1, (d-t)/0.03)
			v := math.Sin(ph) + 0.5*math.Sin(2*ph) + 0.3*math.Sin(3*ph) + 0.2*math.Sin(5*ph)
			seg[i] = float32(0.3 * v * env * (0.6 + 0.4*math.Sin(2*math.Pi*7*t)))
		}
		out = append(out, seg...)
		out = append(out, make(sounds.Clip, int(0.07*sr))...)
	}
	return out
}
