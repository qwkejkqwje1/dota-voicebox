package script

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gameclock"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
)

type fakeHost struct {
	mu    sync.Mutex
	calls []string
	game  Game
}

func (h *fakeHost) rec(s string) { h.mu.Lock(); h.calls = append(h.calls, s); h.mu.Unlock() }
func (h *fakeHost) Play(ref, to string, v float64) error {
	h.rec("play:" + ref + "@" + to)
	return nil
}
func (h *fakeHost) Say(t, to string) error { h.rec("say:" + t + "@" + to); return nil }
func (h *fakeHost) Read(t, to, v string) error {
	h.rec("read:" + t + "@" + to + "/" + v)
	return nil
}
func (h *fakeHost) Stop(ref string)          { h.rec("stop:" + ref) }
func (h *fakeHost) SetPreset(n string) error { h.rec("preset:" + n); return nil }
func (h *fakeHost) Preset() string           { return "clean" }
func (h *fakeHost) Action(a string)          { h.rec("action:" + a) }
func (h *fakeHost) Game() Game               { h.mu.Lock(); defer h.mu.Unlock(); return h.game }
func (h *fakeHost) list() string             { h.mu.Lock(); defer h.mu.Unlock(); return strings.Join(h.calls, ",") }

func wait(t *testing.T, h *fakeHost, want string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if strings.Contains(h.list(), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("want %q, got %q", want, h.list())
}

func setup(t *testing.T, code string) (*Manager, *fakeHost) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "s.js"), []byte(code), 0o644)
	h := &fakeHost{}
	m := NewManager(dir, h)
	m.Sync(map[string]bool{"s": true})
	t.Cleanup(m.Close)
	return m, h
}

func TestDefaultsCompile(t *testing.T) {
	n := map[string]int{}
	for _, dir := range []string{examplesDir(false), examplesDir(true)} {
		entries, _ := defaults.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n[e.Name()]++
			b, _ := defaults.ReadFile(dir + "/" + e.Name())
			if dir == examplesDir(true) && i18n.Cyrillic(string(b)) {
				t.Errorf("%s/%s: русские буквы в английском примере", dir, e.Name())
			}
			h := &fakeHost{}
			s := newScript(e.Name(), string(b), h)
			if err := s.start(); err != nil {
				t.Fatalf("%s/%s: %v", dir, e.Name(), err)
			}
			s.close()
		}
	}
	if len(n) == 0 {
		t.Fatal("no defaults")
	}
	for name, c := range n {
		if c != 2 {
			t.Errorf("%s: пример должен быть и по-русски, и по-английски", name)
		}
	}
}

// Смена языка в мастере: нетронутые примеры — на новом языке, изменённые остаются как есть.
func TestSwapExamples(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "scripts")
	m := NewManager(dir, &fakeHost{})
	t.Cleanup(m.Close)
	m.InstallDefaults() // язык по умолчанию — русский
	mine := filepath.Join(dir, "my_timings.js")
	os.WriteFile(mine, []byte("// моё\n"), 0o644)
	m.SwapExamples(true)
	en, _ := defaults.ReadFile("defaults/en/chat_reader.js")
	if b, _ := os.ReadFile(filepath.Join(dir, "chat_reader.js")); string(b) != string(en) {
		t.Errorf("chat_reader.js не переведён: %.60q", b)
	}
	if b, _ := os.ReadFile(mine); string(b) != "// моё\n" {
		t.Errorf("изменённый пример переписан: %q", b)
	}
	m.SwapExamples(false)
	ru, _ := defaults.ReadFile("defaults/chat_reader.js")
	if b, _ := os.ReadFile(filepath.Join(dir, "chat_reader.js")); string(b) != string(ru) {
		t.Errorf("chat_reader.js не вернулся на русский: %.60q", b)
	}
	if code, ok := Example("chat_reader"); !ok || code != string(ru) {
		t.Errorf("Example: %v %.40q", ok, code)
	}
}

func TestComboAndHold(t *testing.T) {
	m, h := setup(t, `combo("Num1 Num1", () => play("siren", {to:"team"}));
hold("Num2", 50, () => play("x"), () => stop("x"));`)
	t0 := time.Now()
	ev := func(vk uint32, down bool, d time.Duration) { m.Key(combo.Event{VK: vk, Down: down, T: t0.Add(d)}) }
	ev(0x61, true, 0)
	ev(0x61, false, 50*time.Millisecond)
	ev(0x61, true, 200*time.Millisecond)
	wait(t, h, "play:siren@team")
	ev(0x62, true, 0)
	time.Sleep(120 * time.Millisecond)
	wait(t, h, "play:x@")
	ev(0x62, false, time.Second)
	wait(t, h, "stop:x")
}

func TestScheduleAndEvents(t *testing.T) {
	m, h := setup(t, `at("6:00", (t) => play("rune"+t), {before: 10});
every("7:00", (t) => log("w"+t) || play("w"+t), {until:"14:00"});
on("death", () => say("ой"));`)
	snap := func(c int) gameclock.Snapshot { return gameclock.Snapshot{OK: true, Clock: c, Seconds: float64(c)} }
	m.Clock(snap(345))
	m.Clock(snap(350))
	wait(t, h, "play:rune360@")
	m.Clock(snap(420))
	wait(t, h, "play:w420")
	m.Clock(snap(421))
	m.Clock(snap(840))
	wait(t, h, "play:w840")
	m.Event("death", nil)
	wait(t, h, "say:ой@team")
	if n := strings.Count(h.list(), "rune360"); n != 1 {
		t.Fatalf("fired %d times", n)
	}
}

func TestInfiniteLoopInterrupted(t *testing.T) {
	MaxRun = 50 * time.Millisecond
	m, h := setup(t, `on("x", () => { while(true){} }); on("y", () => play("ok"));`)
	m.Event("x", nil)
	m.Event("y", nil)
	wait(t, h, "play:ok")
	l := m.List()
	if len(l) != 1 || !strings.Contains(l[0].Error, "прерван") {
		t.Fatalf("%+v", l)
	}
}

func TestCompileErrorLine(t *testing.T) {
	_, _ = setup(t, "")
	err := Check("a", "let x = 1;\nlet y = ;\n")
	if err == nil || ErrLine(err) != 2 {
		t.Fatalf("%v line=%d", err, ErrLine(err))
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.js"), []byte("\n\nfoo();"), 0o644)
	m := NewManager(dir, &fakeHost{})
	m.Sync(map[string]bool{"bad": true})
	l := m.List()
	if l[0].Running || l[0].ErrLine != 3 {
		t.Fatalf("%+v", l[0])
	}
}

func TestGameRaw(t *testing.T) {
	h := &fakeHost{game: Game{Raw: []byte(`{"hero":{"name":"npc_dota_hero_axe"}}`), Hero: "axe"}}
	s := newScript("g", `on("state", () => play(game.raw.hero.name + "/" + game.hero + "/" + game.clock));`, h)
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	defer s.close()
	s.onEvent("state", nil)
	wait(t, h, "play:npc_dota_hero_axe/axe/null")
}

func TestParseTime(t *testing.T) {
	for in, want := range map[interface{}]int{"6:00": 360, "-1:30": -90, int64(42): 42, "1:00:00": 3600} {
		if got, err := ParseTime(in); err != nil || got != want {
			t.Fatalf("%v → %d %v", in, got, err)
		}
	}
}

func TestChatRead(t *testing.T) {
	m, h := setup(t, `on("chat", (m) => {
  if (m.own || m.wheel) return;
  read(m.ally ? "свои: " + m.text : m.text, { voice: m.ally ? "announcer" : "vile", to: "me" });
});
say("ехидно", { voice: "sly" });`)
	wait(t, h, "read:ехидно@team/sly")
	m.Event("chat", map[string]any{"text": "я свой", "own": true, "ally": true})
	m.Event("chat", map[string]any{"text": "▶ gg", "wheel": true})
	m.Event("chat", map[string]any{"text": "мид ss", "ally": true, "channel": "team"})
	m.Event("chat", map[string]any{"text": "hi", "ally": false, "channel": "all"})
	wait(t, h, "read:hi@me/vile")
	if l := h.list(); !strings.Contains(l, "read:свои: мид ss@me/announcer") || strings.Contains(l, "я свой") || strings.Contains(l, "gg") {
		t.Fatalf("чат: %s", l)
	}
}
