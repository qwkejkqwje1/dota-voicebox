// Package script — пользовательские скрипты на JavaScript (движок goja, без внешних зависимостей).
//
// Каждый скрипт работает в своём изолированном интерпретаторе и своей горутине:
// падение или бесконечный цикл одного скрипта не ломает программу и другие скрипты
// (выполнение обработчика прерывается через MaxRun).
package script

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gameclock"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
)

// MaxRun — сколько может выполняться один обработчик.
var MaxRun = 300 * time.Millisecond

// Host — возможности программы, доступные скриптам.
type Host interface {
	Play(ref, to string, volume float64) error
	Say(text, to string) error
	Stop(ref string)
	SetPreset(name string) error
	Preset() string
	Action(act string)
	Game() Game
}

// Game — состояние игры для скриптов.
type Game struct {
	Clock     gameclock.Snapshot
	Connected bool
	State     string
	Hero      string
	Paused    bool
	Daytime   bool
	Raw       []byte
}

// Hook — что слушает скрипт (для интерфейса).
type Hook struct {
	Kind string `json:"kind"`
	What string `json:"what"`
}

type handler struct {
	fn        goja.Callable
	onRelease goja.Callable
}

type sched struct {
	at, every, from, until, before int
	fn                             goja.Callable
	label                          string
}

type Script struct {
	Name string
	Code string

	host    Host
	vm      *goja.Runtime
	jobs    chan func()
	stop    chan struct{}
	stopped sync.Once
	matcher *combo.Matcher

	// всё ниже трогается только из горутины скрипта
	combos    map[int]handler
	keyFns    map[uint32][]goja.Callable
	scheds    []*sched
	events    map[string][]goja.Callable
	timers    map[int]*time.Timer
	nextTimer int
	lastClock int
	haveClock bool
	fired     map[string]bool
	rawCache  []byte
	rawVal    goja.Value

	mu      sync.Mutex
	hooks   []Hook
	output  []string
	err     string
	errLine int
	dropped bool
}

var lineRe = regexp.MustCompile(`(?:Line |\.js:)(\d+):(\d+)`)

// ErrLine достаёт номер строки из ошибки goja.
func ErrLine(err error) int {
	if err == nil {
		return 0
	}
	if m := lineRe.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

// Check компилирует код без запуска.
func Check(name, code string) error {
	_, err := goja.Compile(name+".js", code, false)
	return err
}

func newScript(name, code string, host Host) *Script {
	return &Script{
		Name: name, Code: code, host: host,
		jobs: make(chan func(), 256), stop: make(chan struct{}),
		matcher: combo.NewMatcher(),
		combos:  map[int]handler{}, keyFns: map[uint32][]goja.Callable{},
		events: map[string][]goja.Callable{}, timers: map[int]*time.Timer{}, fired: map[string]bool{},
	}
}

func (s *Script) print(line string) {
	s.mu.Lock()
	s.output = append(s.output, time.Now().Format("15:04:05 ")+line)
	if len(s.output) > 100 {
		s.output = s.output[len(s.output)-100:]
	}
	s.mu.Unlock()
}

func (s *Script) fail(err error) {
	msg := err.Error()
	if ie, ok := err.(*goja.InterruptedError); ok {
		msg = fmt.Sprintf("обработчик работал дольше %v и был прерван (бесконечный цикл?) %s", MaxRun, strings.TrimPrefix(ie.String(), "timeout"))
	}
	s.mu.Lock()
	s.err, s.errLine = msg, ErrLine(err)
	s.mu.Unlock()
	s.print("ОШИБКА: " + msg)
	log.Printf("Скрипт %s: %s", s.Name, msg)
}

func (s *Script) addHook(kind, what string) {
	s.mu.Lock()
	s.hooks = append(s.hooks, Hook{kind, what})
	s.mu.Unlock()
}

// enqueue ставит задачу в очередь скрипта (не блокирует).
func (s *Script) enqueue(f func()) {
	select {
	case <-s.stop:
		return
	default:
	}
	select {
	case s.jobs <- f:
	default:
		s.mu.Lock()
		d := s.dropped
		s.dropped = true
		s.mu.Unlock()
		if !d {
			log.Printf("Скрипт %s не успевает обрабатывать события — часть пропущена", s.Name)
		}
	}
}

// guard выполняет f с ограничением по времени.
func (s *Script) guard(f func() error) {
	t := time.AfterFunc(MaxRun, func() { s.vm.Interrupt("timeout") })
	err := f()
	t.Stop()
	s.vm.ClearInterrupt()
	if err != nil {
		s.fail(err)
	}
}

func (s *Script) call(fn goja.Callable, args ...interface{}) {
	if fn == nil {
		return
	}
	vals := make([]goja.Value, len(args))
	for i, a := range args {
		vals[i] = s.vm.ToValue(a)
	}
	s.guard(func() error { _, err := fn(goja.Undefined(), vals...); return err })
}

func (s *Script) start() error {
	prog, err := goja.Compile(s.Name+".js", s.Code, false)
	if err != nil {
		s.fail(err)
		return err
	}
	s.vm = goja.New()
	s.install()
	done := make(chan error, 1)
	go s.loop()
	s.enqueue(func() {
		var runErr error
		s.guard(func() error { _, runErr = s.vm.RunProgram(prog); return runErr })
		done <- runErr
	})
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		return fmt.Errorf("запуск скрипта завис")
	}
}

func (s *Script) loop() {
	for {
		select {
		case <-s.stop:
			return
		case f := <-s.jobs:
			func() {
				defer func() {
					if r := recover(); r != nil {
						s.fail(fmt.Errorf("паника: %v", r))
					}
				}()
				f()
			}()
		}
	}
}

func (s *Script) close() {
	s.stopped.Do(func() {
		close(s.stop)
		s.enqueueCleanup()
	})
}

func (s *Script) enqueueCleanup() {
	// таймеры останавливаем без гонки: AfterFunc после stop ничего не сделает
	go func() {
		time.Sleep(10 * time.Millisecond)
		for _, t := range s.timersSnapshot() {
			t.Stop()
		}
	}()
}

func (s *Script) timersSnapshot() []*time.Timer {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*time.Timer, 0, len(s.timers))
	for _, t := range s.timers {
		out = append(out, t)
	}
	return out
}

// ---------- события извне ----------

func (s *Script) onKey(ev combo.Event) {
	for _, f := range s.matcher.Feed(ev) {
		s.dispatch(f)
	}
	s.enqueue(func() {
		for _, fn := range s.keyFns[ev.VK] {
			s.call(fn, ev.Down)
		}
	})
}

func (s *Script) dispatch(f combo.Fired) {
	s.enqueue(func() {
		h := s.combos[f.ID]
		if f.Kind == combo.KindRelease {
			s.call(h.onRelease)
		} else {
			s.call(h.fn)
		}
	})
}

func (s *Script) tick(now time.Time) {
	for _, f := range s.matcher.Tick(now) {
		s.dispatch(f)
	}
}

const maxLate = 3

func (s *Script) onClock(snap gameclock.Snapshot) {
	s.enqueue(func() {
		if !snap.OK {
			return
		}
		c := snap.Clock
		if s.haveClock && c < s.lastClock-5 {
			s.fired = map[string]bool{} // новая игра
		}
		prev := c - 1
		if s.haveClock {
			prev = s.lastClock
		}
		if s.haveClock && c == prev {
			return
		}
		s.lastClock, s.haveClock = c, true
		for i, sc := range s.scheds {
			for _, T := range sc.occ(prev+1+sc.before, c+sc.before) {
				trig := T - sc.before
				key := fmt.Sprintf("%d@%d", i, T)
				if c-trig > maxLate || s.fired[key] {
					continue
				}
				s.fired[key] = true
				s.call(sc.fn, T)
			}
		}
		for _, fn := range s.events["clock"] {
			s.call(fn, c)
		}
	})
}

func (sc *sched) occ(from, to int) []int {
	var out []int
	if sc.every <= 0 {
		if sc.at >= from && sc.at <= to {
			out = append(out, sc.at)
		}
		return out
	}
	k := 0
	if from > sc.from {
		k = (from - sc.from) / sc.every
	}
	for ; ; k++ {
		v := sc.from + k*sc.every
		if v > to || (sc.until != 0 && v > sc.until) {
			break
		}
		if v >= from {
			out = append(out, v)
		}
	}
	return out
}

func (s *Script) onEvent(name string, data interface{}) {
	s.enqueue(func() {
		if name == "new_game" {
			s.fired = map[string]bool{}
			s.haveClock = false
		}
		for _, fn := range s.events[name] {
			s.call(fn, data)
		}
	})
}

// ---------- API для JS ----------

// ParseTime: 360, "6:00", "-1:30", "1:02:03".
func ParseTime(v interface{}) (int, error) {
	switch t := v.(type) {
	case int64:
		return int(t), nil
	case float64:
		return int(math.Round(t)), nil
	case int:
		return t, nil
	case string:
		str := strings.TrimSpace(t)
		neg := strings.HasPrefix(str, "-")
		str = strings.TrimPrefix(str, "-")
		total := 0
		for _, p := range strings.Split(str, ":") {
			n, err := strconv.Atoi(p)
			if err != nil {
				return 0, fmt.Errorf("время %q: нужно вида \"6:00\" или число секунд", t)
			}
			total = total*60 + n
		}
		if neg {
			total = -total
		}
		return total, nil
	}
	return 0, fmt.Errorf("время %v: нужно вида \"6:00\" или число секунд", v)
}

// FmtTime: 360 → "6:00".
func FmtTime(sec int) string {
	sign := ""
	if sec < 0 {
		sign, sec = "-", -sec
	}
	return fmt.Sprintf("%s%d:%02d", sign, sec/60, sec%60)
}

func (s *Script) install() {
	vm := s.vm
	throw := func(format string, a ...interface{}) { panic(vm.NewGoError(fmt.Errorf(format, a...))) }
	fnArg := func(call goja.FunctionCall, i int, what string) goja.Callable {
		fn, ok := goja.AssertFunction(call.Argument(i))
		if !ok {
			throw("%s: аргумент %d должен быть функцией", what, i+1)
		}
		return fn
	}
	opts := func(call goja.FunctionCall, i int) map[string]interface{} {
		v := call.Argument(i)
		if goja.IsUndefined(v) || goja.IsNull(v) {
			return map[string]interface{}{}
		}
		if m, ok := v.Export().(map[string]interface{}); ok {
			return m
		}
		return map[string]interface{}{}
	}
	optStr := func(m map[string]interface{}, k, def string) string {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
		return def
	}
	optNum := func(m map[string]interface{}, k string, def float64) float64 {
		switch v := m[k].(type) {
		case int64:
			return float64(v)
		case float64:
			return v
		}
		return def
	}
	optTime := func(m map[string]interface{}, k string, def int) int {
		v, ok := m[k]
		if !ok || v == nil {
			return def
		}
		n, err := ParseTime(v)
		if err != nil {
			throw("%s: %v", k, err)
		}
		return n
	}
	timeArg := func(call goja.FunctionCall, i int) int {
		n, err := ParseTime(call.Argument(i).Export())
		if err != nil {
			throw("%v", err)
		}
		return n
	}
	str := func(call goja.FunctionCall) string {
		parts := make([]string, len(call.Arguments))
		for i, a := range call.Arguments {
			if o, ok := a.(*goja.Object); ok && o.ClassName() != "Function" {
				if b, err := json.Marshal(a.Export()); err == nil {
					parts[i] = string(b)
					continue
				}
			}
			parts[i] = a.String()
		}
		return strings.Join(parts, " ")
	}

	vm.Set("log", func(call goja.FunctionCall) goja.Value {
		m := str(call)
		s.print(m)
		log.Printf("[%s] %s", s.Name, m)
		return goja.Undefined()
	})
	vm.Set("notify", func(call goja.FunctionCall) goja.Value {
		m := str(call)
		s.print("🔔 " + m)
		log.Printf("🔔 %s", m)
		return goja.Undefined()
	})
	vm.Set("play", func(call goja.FunctionCall) goja.Value {
		o := opts(call, 1)
		if err := s.host.Play(call.Argument(0).String(), optStr(o, "to", ""), optNum(o, "volume", 1)); err != nil {
			s.print("play: " + err.Error())
		}
		return goja.Undefined()
	})
	vm.Set("say", func(call goja.FunctionCall) goja.Value {
		o := opts(call, 1)
		if err := s.host.Say(call.Argument(0).String(), optStr(o, "to", "team")); err != nil {
			s.print("say: " + err.Error())
		}
		return goja.Undefined()
	})
	vm.Set("stop", func(call goja.FunctionCall) goja.Value {
		ref := ""
		if !goja.IsUndefined(call.Argument(0)) {
			ref = call.Argument(0).String()
		}
		s.host.Stop(ref)
		return goja.Undefined()
	})
	vm.Set("preset", func(call goja.FunctionCall) goja.Value {
		if goja.IsUndefined(call.Argument(0)) {
			return vm.ToValue(s.host.Preset())
		}
		if err := s.host.SetPreset(call.Argument(0).String()); err != nil {
			throw("preset: %v", err)
		}
		return vm.ToValue(s.host.Preset())
	})
	vm.Set("action", func(call goja.FunctionCall) goja.Value {
		s.host.Action(call.Argument(0).String())
		return goja.Undefined()
	})
	addCombo := func(kind, pattern string, within time.Duration, h handler) {
		id, err := s.matcher.AddCombo(pattern, within)
		if err != nil {
			throw("%s: %v", kind, err)
		}
		s.combos[id] = h
		s.addHook(kind, pattern)
	}
	vm.Set("hotkey", func(call goja.FunctionCall) goja.Value {
		p := call.Argument(0).String()
		if strings.Contains(strings.TrimSpace(p), " ") {
			throw("hotkey: для последовательностей используйте combo(%q, ...)", p)
		}
		addCombo("hotkey", p, 0, handler{fn: fnArg(call, 1, "hotkey")})
		return goja.Undefined()
	})
	vm.Set("combo", func(call goja.FunctionCall) goja.Value {
		o := opts(call, 2)
		within := time.Duration(optNum(o, "within", 400)) * time.Millisecond
		addCombo("combo", call.Argument(0).String(), within, handler{fn: fnArg(call, 1, "combo")})
		return goja.Undefined()
	})
	vm.Set("hold", func(call goja.FunctionCall) goja.Value {
		p := call.Argument(0).String()
		ms := call.Argument(1).ToInteger()
		h := handler{fn: fnArg(call, 2, "hold")}
		if f, ok := goja.AssertFunction(call.Argument(3)); ok {
			h.onRelease = f
		}
		id, err := s.matcher.AddHold(p, time.Duration(ms)*time.Millisecond)
		if err != nil {
			throw("hold: %v", err)
		}
		s.combos[id] = h
		s.addHook("hold", fmt.Sprintf("%s %dмс", p, ms))
		return goja.Undefined()
	})
	vm.Set("onKey", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		vk, ok := keys.VK(name)
		if !ok {
			throw("onKey: неизвестная клавиша %q", name)
		}
		s.keyFns[vk] = append(s.keyFns[vk], fnArg(call, 1, "onKey"))
		s.addHook("onKey", name)
		return goja.Undefined()
	})
	vm.Set("at", func(call goja.FunctionCall) goja.Value {
		t := timeArg(call, 0)
		o := opts(call, 2)
		sc := &sched{at: t, before: optTime(o, "before", 0), fn: fnArg(call, 1, "at")}
		s.scheds = append(s.scheds, sc)
		s.addHook("at", FmtTime(t))
		return goja.Undefined()
	})
	vm.Set("every", func(call goja.FunctionCall) goja.Value {
		p := timeArg(call, 0)
		if p <= 0 {
			throw("every: период должен быть больше 0")
		}
		o := opts(call, 2)
		sc := &sched{every: p, from: optTime(o, "from", p), until: optTime(o, "until", 0), before: optTime(o, "before", 0), fn: fnArg(call, 1, "every")}
		s.scheds = append(s.scheds, sc)
		s.addHook("every", FmtTime(p))
		return goja.Undefined()
	})
	addTimer := func(call goja.FunctionCall, repeat bool) goja.Value {
		sec := call.Argument(0).ToFloat()
		fn := fnArg(call, 1, "after")
		d := time.Duration(sec * float64(time.Second))
		if d < 50*time.Millisecond {
			d = 50 * time.Millisecond
		}
		s.nextTimer++
		id := s.nextTimer
		var t *time.Timer
		var tick func()
		tick = func() {
			s.enqueue(func() {
				if _, alive := s.timers[id]; !alive {
					return
				}
				if repeat {
					s.mu.Lock()
					s.timers[id] = time.AfterFunc(d, tick)
					s.mu.Unlock()
				} else {
					s.mu.Lock()
					delete(s.timers, id)
					s.mu.Unlock()
				}
				s.call(fn)
			})
		}
		t = time.AfterFunc(d, tick)
		s.mu.Lock()
		s.timers[id] = t
		s.mu.Unlock()
		return vm.ToValue(id)
	}
	vm.Set("after", func(call goja.FunctionCall) goja.Value { return addTimer(call, false) })
	vm.Set("repeat", func(call goja.FunctionCall) goja.Value { return addTimer(call, true) })
	vm.Set("cancel", func(call goja.FunctionCall) goja.Value {
		id := int(call.Argument(0).ToInteger())
		s.mu.Lock()
		if t := s.timers[id]; t != nil {
			t.Stop()
			delete(s.timers, id)
		}
		s.mu.Unlock()
		return goja.Undefined()
	})
	vm.Set("on", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		s.events[name] = append(s.events[name], fnArg(call, 1, "on"))
		s.addHook("on", name)
		return goja.Undefined()
	})
	vm.Set("time", func(call goja.FunctionCall) goja.Value { return vm.ToValue(timeArg(call, 0)) })
	vm.Set("fmt", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(FmtTime(int(math.Floor(call.Argument(0).ToFloat()))))
	})

	game := vm.NewObject()
	prop := func(name string, get func(g Game) interface{}) {
		game.DefineAccessorProperty(name, vm.ToValue(func(goja.FunctionCall) goja.Value {
			return vm.ToValue(get(s.host.Game()))
		}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	}
	prop("clock", func(g Game) interface{} {
		if !g.Clock.OK {
			return nil
		}
		return g.Clock.Clock
	})
	prop("seconds", func(g Game) interface{} {
		if !g.Clock.OK {
			return nil
		}
		return math.Round(g.Clock.Seconds*10) / 10
	})
	prop("clockSource", func(g Game) interface{} { return g.Clock.Source })
	prop("connected", func(g Game) interface{} { return g.Connected })
	prop("running", func(g Game) interface{} { return g.Clock.OK })
	prop("paused", func(g Game) interface{} { return g.Paused })
	prop("daytime", func(g Game) interface{} { return g.Daytime })
	prop("state", func(g Game) interface{} { return g.State })
	prop("hero", func(g Game) interface{} { return g.Hero })
	game.DefineAccessorProperty("raw", vm.ToValue(func(goja.FunctionCall) goja.Value {
		raw := s.host.Game().Raw
		if len(raw) == 0 {
			return goja.Null()
		}
		if string(raw) != string(s.rawCache) {
			var v interface{}
			if json.Unmarshal(raw, &v) != nil {
				return goja.Null()
			}
			s.rawCache, s.rawVal = raw, vm.ToValue(v)
		}
		return s.rawVal
	}), nil, goja.FLAG_FALSE, goja.FLAG_TRUE)
	vm.Set("game", game)
}
