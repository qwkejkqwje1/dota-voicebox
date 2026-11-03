package sounds

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Bus — кто слышит звук.
type Bus int

const (
	BusBoth    Bus = iota // команда (виртуальный кабель) и вы (наушники)
	BusVoice              // только команда
	BusMonitor            // только вы
)

func ParseBus(s string) Bus {
	switch strings.ToLower(s) {
	case "voice", "team":
		return BusVoice
	case "monitor", "local", "me":
		return BusMonitor
	}
	return BusBoth
}

func (b Bus) String() string {
	switch b {
	case BusVoice:
		return "voice"
	case BusMonitor:
		return "monitor"
	}
	return "both"
}

// Режимы повторного нажатия.
const (
	ModeToggle    = "toggle"    // повторное нажатие останавливает звук
	ModeInterrupt = "interrupt" // повторное нажатие запускает звук сначала
	ModeOverlap   = "overlap"   // звуки накладываются
	ModeIgnore    = "ignore"    // пока играет — нажатия игнорируются
)

// Def — описание звука в config.json.
type Def struct {
	Files    []string `json:"files"`              // "sounds/x.mp3", "builtin:siren", "tts:Мид пропал"
	Volume   float64  `json:"volume,omitempty"`   // 0..2, по умолчанию 1
	Bus      string   `json:"bus,omitempty"`      // both | voice | monitor
	Cooldown float64  `json:"cooldown,omitempty"` // сек, защита от спама
	Mode     string   `json:"mode,omitempty"`     // toggle | interrupt | overlap | ignore
	Label    string   `json:"label,omitempty"`    // название на плитке
	Emoji    string   `json:"emoji,omitempty"`    // значок плитки
	Color    string   `json:"color,omitempty"`    // цвет плитки (#rrggbb)
	Category string   `json:"category,omitempty"` // группа на панели
	Seq      bool     `json:"seq,omitempty"`      // играть все файлы подряд (иначе — случайный)
	Dota     bool     `json:"dota,omitempty"`     // часть модуля Dota 2 (скрыт, когда модуль выключен)
	Hidden   bool     `json:"hidden,omitempty"`   // служебный звук, не показывается на панели
}

type Sound struct {
	ID       string
	Label    string
	Source   string // builtin | file | config
	Files    []string
	Clips    []Clip
	Volume   float32
	Bus      Bus
	Cooldown time.Duration
	Mode     string
	Emoji    string
	Color    string
	Category string
	Seq      bool
	Dota     bool
	Hidden   bool
	Pending  bool   // часть фраз ещё синтезируется
	Err      string // почему звук не загрузился (или загрузился не полностью)

	mu   sync.Mutex
	last time.Time
}

// Pick выбирает случайный клип, проверяя кулдаун.
func (s *Sound) Pick() (Clip, bool) {
	if len(s.Clips) == 0 {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Cooldown > 0 && time.Since(s.last) < s.Cooldown {
		return nil, false
	}
	s.last = time.Now()
	return s.Clips[rand.Intn(len(s.Clips))], true
}

// Duration — длительность самого длинного варианта, сек.
func (s *Sound) Duration() float64 {
	var d float64
	for _, c := range s.Clips {
		d = math.Max(d, float64(len(c))/SampleRate)
	}
	return d
}

// TTS — синтезатор речи. Библиотека берёт только готовые фразы из кэша;
// недостающие попадают в Pending(), приложение синтезирует их в фоне
// и перезагружает библиотеку. Так загрузка никогда не ждёт PowerShell.
type TTS interface {
	Cached(text string) (Clip, bool)
}

type Library struct {
	mu      sync.Mutex
	sounds  map[string]*Sound
	pending []string
}

func (l *Library) Get(id string) *Sound {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.sounds[id]
}

func (l *Library) IDs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var ids []string
	for k := range l.sounds {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	return ids
}

// Pending — фразы, которых нет в кэше синтеза речи.
func (l *Library) Pending() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.pending...)
}

// AudioExt — форматы, которые умеет читать библиотека.
var AudioExt = map[string]bool{".wav": true, ".mp3": true}

// SeqGap — пауза между частями звука-последовательности.
const SeqGap = 0.12

// Load собирает библиотеку: встроенные звуки, все файлы из папки sounds/
// (id = имя файла без расширения) и описания из конфига (они главнее).
func Load(baseDir, soundsDir string, defs map[string]Def, tts TTS, normalize bool) *Library {
	lib := &Library{sounds: map[string]*Sound{}}
	for _, name := range BuiltinNames() {
		c, _ := Builtin(name)
		m := builtinMeta[name]
		lib.sounds[name] = &Sound{ID: name, Label: m.label, Emoji: m.emoji, Source: "builtin", Files: []string{"builtin:" + name},
			Clips: []Clip{c}, Volume: 1, Mode: ModeInterrupt}
	}
	if soundsDir != "" {
		dir := soundsDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if e.IsDir() || !AudioExt[ext] {
				continue
			}
			id := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			rel := filepath.ToSlash(filepath.Join(soundsDir, e.Name()))
			s := &Sound{ID: id, Source: "file", Files: []string{rel}, Volume: 1, Mode: ModeToggle}
			c, err := decodeCached(filepath.Join(dir, e.Name()), normalize)
			if err != nil {
				log.Printf("звук %s: %v", e.Name(), err)
				s.Err = err.Error()
			} else {
				s.Clips = []Clip{c}
			}
			lib.sounds[id] = s
		}
	}
	pend := map[string]bool{}
	for id, d := range defs {
		s := &Sound{ID: id, Label: d.Label, Source: "config", Files: d.Files, Volume: 1, Bus: ParseBus(d.Bus), Mode: d.Mode,
			Cooldown: time.Duration(d.Cooldown * float64(time.Second)), Emoji: d.Emoji, Color: d.Color, Category: d.Category,
			Seq: d.Seq, Dota: d.Dota, Hidden: d.Hidden}
		if d.Volume > 0 {
			s.Volume = float32(d.Volume)
		}
		var errs []string
		for _, f := range d.Files {
			if text, ok := strings.CutPrefix(f, "tts:"); ok && tts != nil {
				c, ok := tts.Cached(text)
				if !ok {
					if f, ok := tts.(interface{ Failed(string) error }); ok {
						if err := f.Failed(text); err != nil {
							errs = append(errs, "синтез речи: "+err.Error())
							continue
						}
					}
					s.Pending = true
					if !pend[text] {
						pend[text] = true
						lib.pending = append(lib.pending, text)
					}
					continue
				}
				if normalize {
					c = Normalize(c)
				}
				s.Clips = append(s.Clips, c)
				continue
			}
			c, err := resolve(baseDir, f, normalize)
			if err != nil {
				log.Printf("звук %s (%s): %v", id, f, err)
				errs = append(errs, err.Error())
				continue
			}
			s.Clips = append(s.Clips, c)
		}
		if s.Seq && len(s.Clips) > 1 {
			s.Clips = []Clip{Concat(s.Clips, SeqGap)}
		}
		if len(errs) > 0 {
			s.Err = strings.Join(errs, "; ")
		}
		if len(s.Clips) == 0 && !s.Pending && s.Err == "" {
			s.Err = "нет ни одного файла"
		}
		lib.sounds[id] = s
	}
	sort.Strings(lib.pending)
	return lib
}

// Concat склеивает клипы с паузой gap секунд между ними.
func Concat(clips []Clip, gap float64) Clip {
	g := int(gap * SampleRate)
	n := 0
	for _, c := range clips {
		n += len(c) + g
	}
	out := make(Clip, 0, n)
	for i, c := range clips {
		if i > 0 {
			out = append(out, make(Clip, g)...)
		}
		out = append(out, c...)
	}
	return out
}

func resolve(baseDir, src string, normalize bool) (Clip, error) {
	switch {
	case strings.HasPrefix(src, "builtin:"):
		name := strings.TrimPrefix(src, "builtin:")
		if c, ok := Builtin(name); ok {
			return c, nil
		}
		return nil, fmt.Errorf("нет встроенного звука %q", name)
	case strings.HasPrefix(src, "tts:"):
		return nil, fmt.Errorf("синтез речи недоступен на этой системе")
	}
	p := src
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	return decodeCached(p, normalize)
}

// ---------- кэш декодирования ----------
// Перезагрузка конфига не должна заново декодировать все MP3.

type cacheKey struct {
	path      string
	size      int64
	mtime     int64
	normalize bool
}

var (
	cacheMu sync.Mutex
	cache   = map[string]struct {
		key  cacheKey
		clip Clip
	}{}
)

func decodeCached(path string, normalize bool) (Clip, error) {
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("файл не найден: %s", filepath.Base(path))
		}
		return nil, err
	}
	k := cacheKey{path, st.Size(), st.ModTime().UnixNano(), normalize}
	cacheMu.Lock()
	if e, ok := cache[path]; ok && e.key == k {
		cacheMu.Unlock()
		return e.clip, nil
	}
	cacheMu.Unlock()
	c, err := DecodeFile(path)
	if err != nil {
		return nil, err
	}
	if normalize {
		c = Normalize(c)
	}
	cacheMu.Lock()
	cache[path] = struct {
		key  cacheKey
		clip Clip
	}{k, c}
	cacheMu.Unlock()
	return c, nil
}

// Normalize выравнивает громкость: RMS ≈ -16 dBFS, пик не выше 0.95.
// Убирает тишину в начале (чтобы звук срабатывал сразу по нажатию).
func Normalize(c Clip) Clip {
	start := 0
	for start < len(c) && c[start] < 0.003 && c[start] > -0.003 {
		start++
	}
	if start > SampleRate/2 { // срезаем не больше 0.5 с
		start = SampleRate / 2
	}
	c = append(Clip(nil), c[start:]...)
	var sum float64
	var pk float32
	for _, x := range c {
		sum += float64(x) * float64(x)
		if x < 0 {
			x = -x
		}
		if x > pk {
			pk = x
		}
	}
	if len(c) == 0 || pk == 0 {
		return c
	}
	rms := math.Sqrt(sum / float64(len(c)))
	g := float32(0.158 / rms) // -16 dBFS
	if g*pk > 0.95 {
		g = 0.95 / pk
	}
	for i := range c {
		c[i] *= g
	}
	return c
}

// Info — описание звука для интерфейса.
type Info struct {
	ID       string   `json:"id"`
	Label    string   `json:"label,omitempty"`
	Source   string   `json:"source"`
	Files    []string `json:"files"`
	Duration float64  `json:"duration"`
	Volume   float32  `json:"volume"`
	Bus      string   `json:"bus"`
	Cooldown float64  `json:"cooldown"`
	Mode     string   `json:"mode"`
	Emoji    string   `json:"emoji,omitempty"`
	Color    string   `json:"color,omitempty"`
	Category string   `json:"category,omitempty"`
	Seq      bool     `json:"seq,omitempty"`
	Dota     bool     `json:"dota,omitempty"`
	Hidden   bool     `json:"hidden,omitempty"`
	Pending  bool     `json:"pending,omitempty"`
	Error    string   `json:"error,omitempty"`
}

func (s *Sound) Info() Info {
	return Info{ID: s.ID, Label: s.Label, Source: s.Source, Files: s.Files, Duration: math.Round(s.Duration()*10) / 10,
		Volume: s.Volume, Bus: s.Bus.String(), Cooldown: s.Cooldown.Seconds(), Mode: s.Mode,
		Emoji: s.Emoji, Color: s.Color, Category: s.Category, Seq: s.Seq, Dota: s.Dota, Hidden: s.Hidden,
		Pending: s.Pending, Error: s.Err}
}

func (l *Library) Infos() []Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Info, 0, len(l.sounds))
	for _, s := range l.sounds {
		out = append(out, s.Info())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
