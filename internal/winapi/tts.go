package winapi

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Voice — голос синтеза речи Windows.
type Voice struct {
	ID     string `json:"id"`   // "winrt:Microsoft Irina" | "sapi:Microsoft Irina Desktop"
	Name   string `json:"name"` // для показа
	Lang   string `json:"lang"`
	Gender string `json:"gender,omitempty"`
}

// TTS — синтез речи с кэшем WAV-файлов. Cached не запускает синтез (быстро),
// Render синтезирует пачку фраз одним запуском PowerShell.
type TTS struct {
	Dir   string
	Voice string // Voice.ID; "" — русский голос по умолчанию
	Rate  int    // -10..10

	renderMu sync.Mutex
	mu       sync.Mutex
	mem      map[string]sounds.Clip
	failed   map[string]error
}

func newTTS(dir, voice string, rate int) *TTS {
	return &TTS{Dir: dir, Voice: voice, Rate: rate, mem: map[string]sounds.Clip{}, failed: map[string]error{}}
}

func (t *TTS) path(text string) string {
	h := sha1.Sum([]byte(fmt.Sprintf("v2|%s|%d|%s", t.Voice, t.Rate, text)))
	return filepath.Join(t.Dir, "tts_"+hex.EncodeToString(h[:10])+".wav")
}

// Cached возвращает фразу, если она уже синтезирована.
func (t *TTS) Cached(text string) (sounds.Clip, bool) {
	t.mu.Lock()
	c, ok := t.mem[text]
	t.mu.Unlock()
	if ok {
		return c, true
	}
	p := t.path(text)
	if _, err := os.Stat(p); err != nil {
		return nil, false
	}
	c, err := sounds.DecodeFile(p)
	if err != nil || len(c) == 0 {
		os.Remove(p) // битый файл — синтезируем заново
		return nil, false
	}
	t.mu.Lock()
	t.mem[text] = c
	t.mu.Unlock()
	return c, true
}

// Failed — ошибка последнего синтеза фразы (nil, если не было).
func (t *TTS) Failed(text string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failed[text]
}

// Retry забывает ошибки — следующий Render попробует снова.
func (t *TTS) Retry() {
	t.mu.Lock()
	t.failed = map[string]error{}
	t.mu.Unlock()
}

// Render синтезирует недостающие фразы.
func (t *TTS) Render(texts []string) error {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	var items []ttsItem
	seen := map[string]bool{}
	for _, s := range texts {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if _, ok := t.Cached(s); ok {
			continue
		}
		items = append(items, ttsItem{Text: s, Out: t.path(s)})
	}
	if len(items) == 0 {
		return nil
	}
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return err
	}
	err := t.synth(items)
	var firstErr error
	for _, it := range items {
		if _, ok := t.Cached(it.Text); ok {
			continue
		}
		e := err
		if e == nil {
			e = fmt.Errorf("голос не создал файл")
		}
		t.mu.Lock()
		t.failed[it.Text] = e
		t.mu.Unlock()
		if firstErr == nil {
			firstErr = e
		}
	}
	return firstErr
}

// Say синтезирует (при необходимости) и возвращает одну фразу.
func (t *TTS) Say(text string) (sounds.Clip, error) {
	if c, ok := t.Cached(text); ok {
		return c, nil
	}
	if err := t.Render([]string{text}); err != nil {
		return nil, err
	}
	if c, ok := t.Cached(text); ok {
		return c, nil
	}
	return nil, fmt.Errorf("синтез речи не удался")
}

type ttsItem struct {
	Text string `json:"text"`
	Out  string `json:"out"`
}

// RateWinRT переводит скорость -10..10 в SpeakingRate WinRT (0.5..6, 1 — обычная).
func RateWinRT(r int) float64 {
	if r >= 0 {
		return 1 + float64(r)*0.1
	}
	return 1 + float64(r)*0.05
}
