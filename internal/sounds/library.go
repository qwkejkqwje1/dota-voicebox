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

// Bus — куда играть звук.
type Bus int

const (
	BusBoth    Bus = iota // команде (в войс) и вам в наушники
	BusVoice              // только в войс
	BusMonitor            // только вам (напоминания таймеров)
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

// Def — описание звука в config.json.
type Def struct {
	Files    []string `json:"files"`              // "sounds/x.mp3", "builtin:siren", "tts:Мид пропал"
	Volume   float64  `json:"volume,omitempty"`   // 0..2, по умолчанию 1
	Bus      string   `json:"bus,omitempty"`      // both | voice | monitor
	Cooldown float64  `json:"cooldown,omitempty"` // сек, защита от спама
	Mode     string   `json:"mode,omitempty"`     // overlap | interrupt | ignore
	Label    string   `json:"label,omitempty"`    // подпись кнопки в интерфейсе
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
	last     time.Time
}

// Pick выбирает случайный клип, проверяя кулдаун.
func (s *Sound) Pick() (Clip, bool) {
	if len(s.Clips) == 0 {
		return nil, false
	}
	if s.Cooldown > 0 && time.Since(s.last) < s.Cooldown {
		return nil, false
	}
	s.last = time.Now()
	return s.Clips[rand.Intn(len(s.Clips))], true
}

// TTSFunc синтезирует речь в WAV (реализация для Windows — через System.Speech).
type TTSFunc func(text string) (Clip, error)

type Library struct {
	mu     sync.Mutex
	sounds map[string]*Sound
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
	return ids
}

var audioExt = map[string]bool{".wav": true, ".mp3": true}

// Load собирает библиотеку: встроенные звуки, все файлы из папки sounds/
// (id = имя файла без расширения) и явные описания из конфига (они главнее).
func Load(baseDir, soundsDir string, defs map[string]Def, tts TTSFunc, normalize bool) *Library {
	lib := &Library{sounds: map[string]*Sound{}}
	for _, name := range BuiltinNames() {
		c, _ := Builtin(name)
		lib.sounds[name] = &Sound{ID: name, Source: "builtin", Files: []string{"builtin:" + name}, Clips: []Clip{c}, Volume: 1}
	}
	if soundsDir != "" {
		dir := soundsDir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(baseDir, dir)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if e.IsDir() || !audioExt[ext] {
				continue
			}
			id := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			c, err := decodeCached(filepath.Join(dir, e.Name()), normalize)
			if err != nil {
				log.Printf("звук %s: %v", e.Name(), err)
				continue
			}
			rel := filepath.ToSlash(filepath.Join(soundsDir, e.Name()))
			lib.sounds[id] = &Sound{ID: id, Source: "file", Files: []string{rel}, Clips: []Clip{c}, Volume: 1}
		}
	}
	for id, d := range defs {
		s := &Sound{ID: id, Label: d.Label, Source: "config", Files: d.Files, Volume: 1, Bus: ParseBus(d.Bus), Mode: d.Mode,
			Cooldown: time.Duration(d.Cooldown * float64(time.Second))}
		if d.Volume > 0 {
			s.Volume = float32(d.Volume)
		}
		for _, f := range d.Files {
			c, err := resolve(baseDir, f, tts, normalize)
			if err != nil {
				log.Printf("звук %s (%s): %v", id, f, err)
				continue
			}
			s.Clips = append(s.Clips, c)
		}
		if len(s.Clips) == 0 {
			log.Printf("звук %s: нет ни одного рабочего файла", id)
			continue
		}
		lib.sounds[id] = s
	}
	return lib
}

func resolve(baseDir, src string, tts TTSFunc, normalize bool) (Clip, error) {
	switch {
	case strings.HasPrefix(src, "builtin:"):
		name := strings.TrimPrefix(src, "builtin:")
		if c, ok := Builtin(name); ok {
			return c, nil
		}
		return nil, fmt.Errorf("нет встроенного звука %q (есть: %s)", name, strings.Join(BuiltinNames(), ", "))
	case strings.HasPrefix(src, "tts:"):
		if tts == nil {
			return nil, fmt.Errorf("TTS недоступен на этой системе")
		}
		c, err := tts(strings.TrimPrefix(src, "tts:"))
		if err == nil && normalize {
			c = Normalize(c)
		}
		return c, err
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
}

func (l *Library) Infos() []Info {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Info, 0, len(l.sounds))
	for _, s := range l.sounds {
		var d float64
		for _, c := range s.Clips {
			d = math.Max(d, float64(len(c))/SampleRate)
		}
		out = append(out, Info{ID: s.ID, Label: s.Label, Source: s.Source, Files: s.Files, Duration: math.Round(d*10) / 10,
			Volume: s.Volume, Bus: [...]string{"both", "voice", "monitor"}[s.Bus], Cooldown: s.Cooldown.Seconds(), Mode: s.Mode})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
