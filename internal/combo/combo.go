// Package combo распознаёт комбинации клавиш по потоку нажатий/отпусканий:
//
//	"Num1"            — одиночное нажатие
//	"Ctrl+Num1"       — с модификатором
//	"Num1+Num2"       — держать Num1 и нажать Num2 (аккорд)
//	"Num1 Num1"       — двойное нажатие (последовательность, пробел между шагами)
//	"Num1 Num2 Num3"  — последовательность из нескольких шагов
//
// Отдельно — удержание клавиши N миллисекунд (Hold) и событие отпускания.
package combo

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
)

type Event struct {
	VK   uint32
	Down bool
	T    time.Time
}

type Kind string

const (
	KindCombo   Kind = "combo"
	KindHold    Kind = "hold"
	KindRelease Kind = "release"
)

type Fired struct {
	ID   int
	Kind Kind
}

// группы модификаторов: общий VK + левый/правый
var modGroups = map[string][]uint32{
	"CTRL": {0x11, 0xA2, 0xA3}, "CONTROL": {0x11, 0xA2, 0xA3},
	"SHIFT": {0x10, 0xA0, 0xA1}, "ALT": {0x12, 0xA4, 0xA5}, "WIN": {0x5B, 0x5C},
}

func isModifier(vk uint32) bool {
	for _, g := range modGroups {
		for _, v := range g {
			if v == vk {
				return true
			}
		}
	}
	return false
}

type step struct {
	held [][]uint32 // каждая группа — «любая из этих клавиш зажата»
	key  uint32
}

func parseKey(name string) ([]uint32, error) {
	if g, ok := modGroups[strings.ToUpper(name)]; ok {
		return g, nil
	}
	if v, ok := keys.VK(name); ok {
		return []uint32{v}, nil
	}
	return nil, fmt.Errorf("неизвестная клавиша %q", name)
}

func parseStep(s string) (step, error) {
	parts := strings.Split(s, "+")
	var st step
	for i, p := range parts {
		g, err := parseKey(strings.TrimSpace(p))
		if err != nil {
			return st, err
		}
		if i == len(parts)-1 {
			st.key = g[len(g)-1]
			if len(g) > 1 { // модификатор как последняя клавиша — берём общий код
				st.key = g[0]
			}
		} else {
			st.held = append(st.held, g)
		}
	}
	return st, nil
}

// Parse разбирает шаблон комбинации.
func Parse(pattern string) ([]step, error) {
	fields := strings.Fields(pattern)
	if len(fields) == 0 {
		return nil, fmt.Errorf("пустая комбинация")
	}
	var out []step
	for _, f := range fields {
		st, err := parseStep(f)
		if err != nil {
			return nil, fmt.Errorf("%q: %w", pattern, err)
		}
		out = append(out, st)
	}
	return out, nil
}

type seq struct {
	id     int
	steps  []step
	within time.Duration
	prog   int
	last   time.Time
}

type hold struct {
	id      int
	st      step
	dur     time.Duration
	started time.Time
	active  bool
	fired   bool
}

// Matcher — потокобезопасный распознаватель.
type Matcher struct {
	mu    sync.Mutex
	down  map[uint32]bool
	seqs  []*seq
	holds []*hold
	next  int
}

func NewMatcher() *Matcher { return &Matcher{down: map[uint32]bool{}} }

// AddCombo регистрирует комбинацию; within — максимум между шагами последовательности.
func (m *Matcher) AddCombo(pattern string, within time.Duration) (int, error) {
	steps, err := Parse(pattern)
	if err != nil {
		return 0, err
	}
	if within <= 0 {
		within = 400 * time.Millisecond
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	m.seqs = append(m.seqs, &seq{id: m.next, steps: steps, within: within})
	return m.next, nil
}

// AddHold — удержание клавиши (можно с модификатором) не меньше dur.
func (m *Matcher) AddHold(pattern string, dur time.Duration) (int, error) {
	steps, err := Parse(pattern)
	if err != nil {
		return 0, err
	}
	if len(steps) != 1 {
		return 0, fmt.Errorf("удержание: нужна одна клавиша, а не последовательность")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	m.holds = append(m.holds, &hold{id: m.next, st: steps[0], dur: dur})
	return m.next, nil
}

func (m *Matcher) isDown(vk uint32) bool {
	if m.down[vk] {
		return true
	}
	// общий код модификатора: 0x11 зажат, если зажат левый или правый
	for _, g := range modGroups {
		if g[0] == vk {
			for _, v := range g[1:] {
				if m.down[v] {
					return true
				}
			}
		}
	}
	return false
}

func (m *Matcher) matches(vk uint32, st step) bool {
	keyOK := vk == st.key
	if !keyOK {
		for _, g := range modGroups {
			if g[0] == st.key {
				for _, v := range g {
					if v == vk {
						keyOK = true
					}
				}
			}
		}
	}
	if !keyOK {
		return false
	}
	// лишние модификаторы запрещены: "Num1" не срабатывает на Ctrl+Num1
	for _, fam := range []uint32{0x11, 0x10, 0x12, 0x5B} {
		if m.familyDown(fam) && !stepUses(st, fam) && !sameFamily(st.key, fam) {
			return false
		}
	}
	for _, g := range st.held {
		any := false
		for _, v := range g {
			if m.isDown(v) {
				any = true
			}
		}
		if !any {
			return false
		}
	}
	return true
}

// Feed обрабатывает событие клавиатуры/мыши.
func (m *Matcher) Feed(e Event) []Fired {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Fired
	if !e.Down {
		delete(m.down, e.VK)
		for _, h := range m.holds {
			if h.active && h.st.key == e.VK {
				if h.fired {
					out = append(out, Fired{h.id, KindRelease})
				}
				h.active, h.fired = false, false
			}
		}
		return out
	}
	if m.down[e.VK] {
		return nil // автоповтор
	}
	m.down[e.VK] = true
	for _, s := range m.seqs {
		if s.prog > 0 && e.T.Sub(s.last) > s.within {
			s.prog = 0
		}
		switch {
		case m.matches(e.VK, s.steps[s.prog]):
			s.prog++
			s.last = e.T
		case m.matches(e.VK, s.steps[0]):
			s.prog, s.last = 1, e.T
		case isModifier(e.VK):
			// нажатие модификатора не сбрасывает прогресс
		default:
			s.prog = 0
		}
		if s.prog == len(s.steps) {
			s.prog = 0
			out = append(out, Fired{s.id, KindCombo})
		}
	}
	for _, h := range m.holds {
		if m.matches(e.VK, h.st) {
			h.active, h.fired, h.started = true, false, e.T
		}
	}
	return out
}

// Tick проверяет удержания; вызывать ~каждые 20–50 мс.
func (m *Matcher) Tick(now time.Time) []Fired {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Fired
	for _, h := range m.holds {
		if h.active && !h.fired && now.Sub(h.started) >= h.dur {
			h.fired = true
			out = append(out, Fired{h.id, KindHold})
		}
	}
	return out
}

// Reset — забыть состояние (например, после потери фокуса/паузы перехвата).
func (m *Matcher) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.down = map[uint32]bool{}
	for _, s := range m.seqs {
		s.prog = 0
	}
	for _, h := range m.holds {
		h.active, h.fired = false, false
	}
}

func family(vk uint32) []uint32 {
	for _, g := range modGroups {
		if g[0] == vk {
			return g
		}
	}
	return nil
}

func sameFamily(vk, fam uint32) bool {
	for _, v := range family(fam) {
		if v == vk {
			return true
		}
	}
	return false
}

func (m *Matcher) familyDown(fam uint32) bool {
	for _, v := range family(fam) {
		if m.down[v] {
			return true
		}
	}
	return false
}

func stepUses(st step, fam uint32) bool {
	for _, g := range st.held {
		for _, v := range g {
			if sameFamily(v, fam) {
				return true
			}
		}
	}
	return false
}
