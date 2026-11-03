// Package audio — микшер звуков и аудио-движок (микрофон → эффекты → кабель).
package audio

import (
	"sync"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

type voice struct {
	id       string
	clip     sounds.Clip
	pos      int
	gain     float32
	stopping bool // затухает и будет удалён
	fade     int  // сколько сэмплов затухания осталось
	fadeN    int
}

const (
	fadeStop    = sounds.SampleRate * 40 / 1000 // остановка без щелчка
	fadeRestart = sounds.SampleRate * 8 / 1000  // перезапуск того же звука
)

// Mixer хранит играющие звуки для одной шины (voice или monitor).
type Mixer struct {
	mu     sync.Mutex
	voices []*voice
}

// Add запускает звук. mode: overlap (наложить), interrupt/toggle (сначала), ignore (если уже играет — не трогать).
// Логику «повторное нажатие останавливает» (toggle) проверяет вызывающий через Has.
func (m *Mixer) Add(id string, c sounds.Clip, gain float32, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch mode {
	case "interrupt", "toggle":
		m.fadeLocked(id, fadeRestart)
	case "ignore":
		for _, v := range m.voices {
			if v.id == id && !v.stopping {
				return
			}
		}
	}
	m.voices = append(m.voices, &voice{id: id, clip: c, gain: gain})
}

func (m *Mixer) fadeLocked(id string, n int) {
	for _, v := range m.voices {
		if (id == "" || v.id == id) && !v.stopping {
			v.stopping, v.fade, v.fadeN = true, n, n
		}
	}
}

// Stop плавно (40 мс) останавливает звук id или все звуки (id == "").
func (m *Mixer) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fadeLocked(id, fadeStop)
}

// Clear мгновенно убирает все звуки (при остановке движка).
func (m *Mixer) Clear() {
	m.mu.Lock()
	m.voices = nil
	m.mu.Unlock()
}

// Has — играет ли звук id (не считая затухающих).
func (m *Mixer) Has(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.voices {
		if v.id == id && !v.stopping {
			return true
		}
	}
	return false
}

func (m *Mixer) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.voices) > 0
}

// Playing — что играет сейчас (для полосок прогресса на плитках).
type Playing struct {
	ID  string  `json:"id"`
	Pos float64 `json:"pos"` // сек от начала
	Len float64 `json:"len"` // длительность, сек
}

// Playing возвращает снимок играющих звуков (без затухающих).
func (m *Mixer) Playing() []Playing {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Playing
	for _, v := range m.voices {
		if !v.stopping {
			out = append(out, Playing{ID: v.id, Pos: float64(v.pos) / sounds.SampleRate, Len: float64(len(v.clip)) / sounds.SampleRate})
		}
	}
	return out
}

// MergePlaying объединяет снимки шин: один звук — одна запись (с наибольшей позицией).
func MergePlaying(lists ...[]Playing) []Playing {
	var out []Playing
	idx := map[string]int{}
	for _, l := range lists {
		for _, p := range l {
			if i, ok := idx[p.ID]; ok {
				if p.Pos > out[i].Pos {
					out[i] = p
				}
				continue
			}
			idx[p.ID] = len(out)
			out = append(out, p)
		}
	}
	return out
}

// Render добавляет (+=) звуки в out. Возвращает true, если что-то играло.
func (m *Mixer) Render(out []float32) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.voices) == 0 {
		return false
	}
	live := m.voices[:0]
	for _, v := range m.voices {
		if v.stopping {
			n := min(len(out), len(v.clip)-v.pos, v.fade)
			for i := 0; i < n; i++ {
				g := v.gain * float32(v.fade-i) / float32(v.fadeN)
				out[i] += v.clip[v.pos+i] * g
			}
			v.pos += n
			v.fade -= n
			if v.fade > 0 && v.pos < len(v.clip) {
				live = append(live, v)
			}
			continue
		}
		n := copyAdd(out, v.clip[v.pos:], v.gain)
		v.pos += n
		if v.pos < len(v.clip) {
			live = append(live, v)
		}
	}
	for i := len(live); i < len(m.voices); i++ {
		m.voices[i] = nil
	}
	m.voices = live
	return true
}

func copyAdd(dst, src []float32, g float32) int {
	n := len(dst)
	if len(src) < n {
		n = len(src)
	}
	for i := 0; i < n; i++ {
		dst[i] += src[i] * g
	}
	return n
}

// softClip — мягкое ограничение итогового сигнала.
func softClip(b []float32) {
	for i, x := range b {
		switch {
		case x > 1:
			b[i] = 1
		case x < -1:
			b[i] = -1
		case x > 0.8:
			b[i] = 0.8 + (x-0.8)*0.5
		case x < -0.8:
			b[i] = -0.8 + (x+0.8)*0.5
		}
	}
}

// ring — простой кольцевой буфер (обработанный микрофон → наушники для самопрослушки).
type ring struct {
	mu   sync.Mutex
	buf  []float32
	r, w int
	n    int
}

func newRing(size int) *ring { return &ring{buf: make([]float32, size)} }

func (q *ring) Write(p []float32) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, x := range p {
		q.buf[q.w] = x
		q.w = (q.w + 1) % len(q.buf)
		if q.n < len(q.buf) {
			q.n++
		} else {
			q.r = (q.r + 1) % len(q.buf)
		}
	}
}

// ReadAdd добавляет доступные сэмплы в out.
func (q *ring) ReadAdd(out []float32) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := range out {
		if q.n == 0 {
			return
		}
		out[i] += q.buf[q.r]
		q.r = (q.r + 1) % len(q.buf)
		q.n--
	}
}

func (q *ring) Clear() {
	q.mu.Lock()
	q.r, q.w, q.n = 0, 0, 0
	q.mu.Unlock()
}
