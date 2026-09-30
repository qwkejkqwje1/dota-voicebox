// Package timers — таймеры по игровому времени Dota 2 (clock_time из GSI):
// руны, Рошан, стаки и любые свои события.
package timers

import (
	"fmt"
	"sort"
)

// Timer описывает повторяющееся или разовое событие.
//
//	at: [360]                       — разовые моменты (секунды игрового времени)
//	start: 420, every: 420          — повторять с 7:00 каждые 7 минут
//	until: 0                        — без ограничения
//	warn_before: 15                 — сыграть звук за 15 секунд до события
type Timer struct {
	ID         string `json:"id"`
	Sound      string `json:"sound"`
	At         []int  `json:"at,omitempty"`
	Start      int    `json:"start,omitempty"`
	Every      int    `json:"every,omitempty"`
	Until      int    `json:"until,omitempty"`
	Skip       []int  `json:"skip,omitempty"` // исключить моменты (например 360 у силовых рун, если есть отдельный таймер)
	WarnBefore int    `json:"warn_before,omitempty"`
	Disabled   bool   `json:"disabled,omitempty"`
}

// Fire — сработавший таймер.
type Fire struct {
	TimerID string
	Sound   string
	EventAt int // момент события (не момент предупреждения)
}

// Engine отслеживает время и выдаёт срабатывания ровно один раз.
type Engine struct {
	timers  []Timer
	oneshot []Timer
	last    int
	have    bool
	fired   map[string]bool
	// MaxLate — насколько поздно (сек) ещё можно сыграть пропущенное событие
	// (например, после alt-tab или переподключения).
	MaxLate int
}

func New(ts []Timer) *Engine {
	return &Engine{timers: ts, fired: map[string]bool{}, MaxLate: 3}
}

func (e *Engine) SetTimers(ts []Timer) { e.timers = ts }

// AddOneShot добавляет разовое событие (например, окно Рошана).
func (e *Engine) AddOneShot(id, sound string, at, warn int) {
	e.oneshot = append(e.oneshot, Timer{ID: id, Sound: sound, At: []int{at}, WarnBefore: warn})
}

// Reset — новая игра.
func (e *Engine) Reset() {
	e.fired = map[string]bool{}
	e.oneshot = nil
	e.have = false
}

func occurrences(t Timer, from, to int) []int {
	var out []int
	for _, a := range t.At {
		if a >= from && a <= to {
			out = append(out, a)
		}
	}
	if t.Every > 0 {
		k := (from - t.Start) / t.Every
		if k < 0 {
			k = 0
		}
		for ; ; k++ {
			v := t.Start + k*t.Every
			if v > to || (t.Until > 0 && v > t.Until) {
				break
			}
			if v >= from {
				out = append(out, v)
			}
		}
	}
	res := out[:0]
	for _, v := range out {
		skip := false
		for _, s := range t.Skip {
			if s == v {
				skip = true
			}
		}
		if !skip {
			res = append(res, v)
		}
	}
	return res
}

// Update вызывается на каждом GSI-тике с текущим clock_time.
func (e *Engine) Update(clock int) []Fire {
	if e.have && clock < e.last-5 { // время ушло назад — новая игра/реплей
		e.Reset()
	}
	prev := clock - 1
	if e.have {
		prev = e.last
	}
	e.last, e.have = clock, true
	if clock == prev {
		return nil
	}
	var out []Fire
	check := func(t Timer) {
		if t.Disabled || t.Sound == "" {
			return
		}
		// событие T срабатывает, когда prev < T-warn <= clock
		for _, T := range occurrences(t, prev+1+t.WarnBefore, clock+t.WarnBefore) {
			trig := T - t.WarnBefore
			if clock-trig > e.MaxLate {
				continue
			}
			key := fmt.Sprintf("%s@%d", t.ID, T)
			if e.fired[key] {
				continue
			}
			e.fired[key] = true
			out = append(out, Fire{TimerID: t.ID, Sound: t.Sound, EventAt: T})
		}
	}
	for _, t := range e.timers {
		check(t)
	}
	for _, t := range e.oneshot {
		check(t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EventAt < out[j].EventAt })
	return out
}

// Clock возвращает последнее известное игровое время.
func (e *Engine) Clock() (int, bool) { return e.last, e.have }
