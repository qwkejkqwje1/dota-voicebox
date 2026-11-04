package winapi

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

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
// Render синтезирует пачку фраз (в Windows — постоянным процессом PowerShell, см. tts_windows.go).
type TTS struct {
	Dir   string
	Voice string // Voice.ID; "" — голос языка Lang по умолчанию
	Rate  int    // -10..10
	Lang  string // язык голоса по умолчанию: "ru" (по умолчанию) | "en"

	renderMu sync.Mutex
	mu       sync.Mutex
	mem      map[string]sounds.Clip
	failed   map[string]error
}

// SpeakOpts — голос одной фразы (для «Читалки»: у персонажей своя скорость и пол голоса).
type SpeakOpts struct {
	Voice  string // Voice.ID; "" — голос языка Lang (и пола Gender)
	Rate   int    // -10..10
	Gender string // Male | Female — голос нужного пола, если голос не выбран явно
	Lang   string // язык фразы ("" — как у синтезатора): чат на английском читает английский голос, если он есть
	Prefer string // голос, если подходит по языку и полу (голос фраз из настроек)
	Temp   bool   // не хранить в кэше: сообщения «Читалки» и чата звучат один раз
}

func newTTS(dir, voice string, rate int) *TTS {
	return &TTS{Dir: dir, Voice: voice, Rate: rate, mem: map[string]sounds.Clip{}, failed: map[string]error{}}
}

func (t *TTS) base() SpeakOpts { return SpeakOpts{Voice: t.Voice, Rate: t.Rate} }

func (t *TTS) lang() string {
	if t.Lang == "" {
		return "ru"
	}
	return t.Lang
}

// memKey — ключ кэша в памяти: у фраз с голосом из настроек — сам текст.
func (t *TTS) memKey(text string, o SpeakOpts) string {
	if o == t.base() {
		return text
	}
	return fmt.Sprintf("\x00%s|%d|%s|%s|%s|%s", o.Voice, o.Rate, o.Gender, o.Lang, o.Prefer, text)
}

func (t *TTS) path(text string) string { return t.pathFor(text, t.base()) }

func (t *TTS) pathFor(text string, o SpeakOpts) string {
	lang := ""
	if t.lang() != "ru" {
		lang = t.lang() + "|"
	}
	key := fmt.Sprintf("v2|%s%s|%d|%s", lang, o.Voice, o.Rate, text) // старый ключ: кэш фраз не пропадает
	if o != t.base() {
		key = fmt.Sprintf("v3|%s%s|%d|%s|%s|%s|%s", lang, o.Voice, o.Rate, o.Gender, o.Lang, o.Prefer, text)
	}
	h := sha1.Sum([]byte(key))
	return filepath.Join(t.Dir, "tts_"+hex.EncodeToString(h[:10])+".wav")
}

// Cached возвращает фразу, если она уже синтезирована.
func (t *TTS) Cached(text string) (sounds.Clip, bool) { return t.CachedWith(text, t.base()) }

// CachedWith — то же для фразы со своим голосом.
func (t *TTS) CachedWith(text string, o SpeakOpts) (sounds.Clip, bool) {
	k := t.memKey(text, o)
	t.mu.Lock()
	c, ok := t.mem[k]
	t.mu.Unlock()
	if ok {
		return c, true
	}
	p := t.pathFor(text, o)
	if _, err := os.Stat(p); err != nil {
		return nil, false
	}
	c, err := sounds.DecodeFile(p)
	if err != nil || len(c) == 0 {
		os.Remove(p) // битый файл — синтезируем заново
		return nil, false
	}
	t.mu.Lock()
	t.mem[k] = c
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
	_, err := t.render(texts, t.base())
	return err
}

func (t *TTS) item(text string, o SpeakOpts) ttsItem {
	lang := o.Lang
	if lang == "" {
		lang = t.lang()
		if lang != "ru" && cyrillic(text) { // английский интерфейс, а фраза русская — русским голосом
			lang = "ru"
		}
	}
	return ttsItem{Text: text, Out: t.pathFor(text, o), Voice: o.Voice, Rate: o.Rate, WinRTRate: RateWinRT(o.Rate),
		Gender: o.Gender, Lang: lang, Prefer: o.Prefer}
}

func (t *TTS) render(texts []string, o SpeakOpts) (int, error) {
	t.renderMu.Lock()
	defer t.renderMu.Unlock()
	var items []ttsItem
	seen := map[string]bool{}
	for _, s := range texts {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		if _, ok := t.CachedWith(s, o); ok {
			continue
		}
		items = append(items, t.item(s, o))
	}
	if len(items) == 0 {
		return 0, nil
	}
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return 0, err
	}
	err := t.synth(items)
	var firstErr error
	for _, it := range items {
		if _, ok := t.CachedWith(it.Text, o); ok {
			continue
		}
		e := err
		if e == nil {
			e = fmt.Errorf("голос не создал файл")
		}
		if o == t.base() {
			t.mu.Lock()
			t.failed[it.Text] = e
			t.mu.Unlock()
		}
		if firstErr == nil {
			firstErr = e
		}
	}
	return len(items), firstErr
}

// Say синтезирует (при необходимости) и возвращает одну фразу.
func (t *TTS) Say(text string) (sounds.Clip, error) { return t.SayWith(text, t.base()) }

// SayWith — фраза своим голосом и скоростью (голос персонажа в «Читалке»).
func (t *TTS) SayWith(text string, o SpeakOpts) (sounds.Clip, error) {
	if o.Temp {
		return t.sayTemp(text, o)
	}
	if c, ok := t.CachedWith(text, o); ok {
		return c, nil
	}
	if _, err := t.render([]string{text}, o); err != nil {
		return nil, err
	}
	if c, ok := t.CachedWith(text, o); ok {
		return c, nil
	}
	return nil, fmt.Errorf("синтез речи не удался")
}

// sayTemp синтезирует фразу во временный файл и сразу его удаляет (кэш не растёт от сообщений чата).
// Фоновый синтез библиотеки не ждёт: постоянный процесс берёт фразы по одной.
func (t *TTS) sayTemp(text string, o SpeakOpts) (sounds.Clip, error) {
	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return nil, err
	}
	it := t.item(text, o)
	it.Out = filepath.Join(t.Dir, fmt.Sprintf("tts_tmp_%d.wav", tempSeq.Add(1)+time.Now().UnixNano()))
	err := t.synth([]ttsItem{it})
	defer os.Remove(it.Out)
	c, derr := sounds.DecodeFile(it.Out)
	if derr != nil || len(c) == 0 {
		if err == nil {
			err = fmt.Errorf("голос не создал файл")
		}
		return nil, err
	}
	return c, nil
}

var tempSeq atomic.Int64

// ttsItem — одна фраза для PowerShell (у каждой может быть свой голос).
type ttsItem struct {
	ID        string  `json:"id,omitempty"` // номер запроса к постоянному процессу
	Text      string  `json:"text"`
	Out       string  `json:"out"`
	Voice     string  `json:"voice"`
	Rate      int     `json:"rate"`
	WinRTRate float64 `json:"winrt_rate"`
	Gender    string  `json:"gender,omitempty"`
	Lang      string  `json:"lang"`
	Prefer    string  `json:"prefer,omitempty"`
}

// RateWinRT переводит скорость -10..10 в SpeakingRate WinRT (0.5..6, 1 — обычная).
func RateWinRT(r int) float64 {
	if r >= 0 {
		return 1 + float64(r)*0.1
	}
	return 1 + float64(r)*0.05
}

func cyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}
