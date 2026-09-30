// Package audio — микшер звуков и аудио-движок (микрофон → эффекты → кабель).
package audio

import (
	"sync"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

type voice struct {
	id   string
	clip sounds.Clip
	pos  int
	gain float32
}

// Mixer хранит играющие звуки для одной шины (voice или monitor).
type Mixer struct {
	mu     sync.Mutex
	voices []*voice
}

func (m *Mixer) Add(id string, c sounds.Clip, gain float32, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch mode {
	case "interrupt":
		m.removeLocked(id)
	case "ignore":
		for _, v := range m.voices {
			if v.id == id {
				return
			}
		}
	}
	m.voices = append(m.voices, &voice{id: id, clip: c, gain: gain})
}

func (m *Mixer) removeLocked(id string) {
	out := m.voices[:0]
	for _, v := range m.voices {
		if v.id != id {
			out = append(out, v)
		}
	}
	m.voices = out
}

func (m *Mixer) Stop(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id == "" {
		m.voices = nil
		return
	}
	m.removeLocked(id)
}

func (m *Mixer) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.voices) > 0
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
		n := copyAdd(out, v.clip[v.pos:], v.gain)
		v.pos += n
		if v.pos < len(v.clip) {
			live = append(live, v)
		}
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
