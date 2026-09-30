package sounds

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
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
}

type Sound struct {
	ID       string
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
func Load(baseDir, soundsDir string, defs map[string]Def, tts TTSFunc) *Library {
	lib := &Library{sounds: map[string]*Sound{}}
	for _, name := range BuiltinNames() {
		c, _ := Builtin(name)
		lib.sounds[name] = &Sound{ID: name, Clips: []Clip{c}, Volume: 1}
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
			c, err := DecodeFile(filepath.Join(dir, e.Name()))
			if err != nil {
				log.Printf("звук %s: %v", e.Name(), err)
				continue
			}
			lib.sounds[id] = &Sound{ID: id, Clips: []Clip{c}, Volume: 1}
		}
	}
	for id, d := range defs {
		s := &Sound{ID: id, Volume: 1, Bus: ParseBus(d.Bus), Mode: d.Mode,
			Cooldown: time.Duration(d.Cooldown * float64(time.Second))}
		if d.Volume > 0 {
			s.Volume = float32(d.Volume)
		}
		for _, f := range d.Files {
			c, err := resolve(baseDir, f, tts)
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

func resolve(baseDir, src string, tts TTSFunc) (Clip, error) {
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
		return tts(strings.TrimPrefix(src, "tts:"))
	}
	p := src
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	return DecodeFile(p)
}
