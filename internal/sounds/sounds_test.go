package sounds

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinsAndWAVRoundtrip(t *testing.T) {
	for _, n := range BuiltinNames() {
		c, _ := Builtin(n)
		if len(c) == 0 {
			t.Fatalf("%s пустой", n)
		}
		back, err := DecodeWAV(EncodeWAV16(c))
		if err != nil || len(back) != len(c) {
			t.Fatalf("%s: %v %d/%d", n, err, len(back), len(c))
		}
	}
	r := Resample(make([]float32, 22050), 22050, 48000)
	if len(r) != 48000 {
		t.Fatal(len(r))
	}
}

func TestLibraryDropIn(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "sounds"), 0o755)
	c, _ := Builtin("beep")
	os.WriteFile(filepath.Join(dir, "sounds", "my_gank.wav"), EncodeWAV16(c), 0o644)
	lib := Load(dir, "sounds", map[string]Def{
		"alarm": {Files: []string{"builtin:siren", "sounds/my_gank.wav"}, Bus: "voice", Cooldown: 5},
	}, nil, true)
	if lib.Get("my_gank") == nil || lib.Get("siren") == nil {
		t.Fatal("нет автоподхваченного или встроенного звука")
	}
	a := lib.Get("alarm")
	if a == nil || len(a.Clips) != 2 || a.Bus != BusVoice {
		t.Fatalf("%+v", a)
	}
	if _, ok := a.Pick(); !ok {
		t.Fatal("pick")
	}
	if _, ok := a.Pick(); ok {
		t.Fatal("кулдаун не сработал")
	}
}

func TestNormalize(t *testing.T) {
	c := make(Clip, 48000)
	for i := 1000; i < len(c); i++ {
		c[i] = 0.01
		if i%2 == 0 {
			c[i] = -0.01
		}
	}
	n := Normalize(c)
	if len(n) != 47000 || (n[0] < 0.1 && n[0] > -0.1) {
		t.Fatalf("len=%d first=%v", len(n), n[0])
	}
}

func TestBuiltinsClean(t *testing.T) {
	for _, n := range BuiltinNames() {
		c, _ := Builtin(n)
		if float64(len(c))/SampleRate < 0.1 {
			t.Errorf("%s слишком короткий", n)
		}
		var pk float32
		for _, x := range c {
			if x != x {
				t.Fatalf("%s: NaN", n)
			}
			if x < 0 {
				x = -x
			}
			if x > pk {
				pk = x
			}
		}
		if pk > 0.95 || pk < 0.3 {
			t.Errorf("%s: пик %.2f", n, pk)
		}
		// конец без щелчка
		for _, x := range c[len(c)-48:] {
			if x > 0.05 || x < -0.05 {
				t.Errorf("%s: обрывается на %.3f", n, x)
				break
			}
		}
	}
	if c1, _ := Builtin("airhorn"); len(c1) > 0 {
		c2, _ := Builtin("airhorn")
		if &c1[0] != &c2[0] {
			t.Error("встроенные звуки должны кэшироваться")
		}
	}
	if len(BuiltinInfos()) != len(BuiltinNames()) {
		t.Error("BuiltinInfos")
	}
	for _, b := range BuiltinInfos() {
		if b.Label == b.ID || b.Emoji == "" {
			t.Errorf("нет подписи для %s", b.ID)
		}
	}
}

type fakeTTS map[string]Clip

func (f fakeTTS) Cached(text string) (Clip, bool) { c, ok := f[text]; return c, ok }

func TestSeqTTSPendingAndErrors(t *testing.T) {
	beep, _ := Builtin("beep")
	defs := map[string]Def{
		"gank":   {Files: []string{"builtin:beep", "tts:Мид, ганк!"}, Seq: true, Emoji: "🚨", Dota: true},
		"broken": {Files: []string{"sounds/nope.mp3"}},
	}
	tts := fakeTTS{}
	lib := Load(t.TempDir(), "sounds", defs, tts, false)
	g := lib.Get("gank")
	if g == nil || !g.Pending || len(g.Clips) != 1 || len(g.Clips[0]) != len(beep) {
		t.Fatalf("до синтеза: %+v", g)
	}
	if p := lib.Pending(); len(p) != 1 || p[0] != "Мид, ганк!" {
		t.Fatalf("pending %v", p)
	}
	b := lib.Get("broken")
	if b == nil || b.Err == "" {
		t.Fatalf("сломанный звук должен остаться с ошибкой: %+v", b)
	}
	if _, ok := b.Pick(); ok {
		t.Fatal("сломанный звук не должен играть")
	}
	tts["Мид, ганк!"] = make(Clip, 4800)
	lib = Load(t.TempDir(), "sounds", defs, tts, false)
	g = lib.Get("gank")
	want := len(beep) + int(SeqGap*SampleRate) + 4800
	if g.Pending || len(g.Clips) != 1 || len(g.Clips[0]) != want || len(lib.Pending()) != 0 {
		t.Fatalf("после синтеза: pending=%v clips=%d len=%d want %d", g.Pending, len(g.Clips), len(g.Clips[0]), want)
	}
	inf := g.Info()
	if !inf.Dota || inf.Emoji != "🚨" || !inf.Seq || inf.Bus != "both" {
		t.Fatalf("%+v", inf)
	}
	// без TTS вообще — ошибка, а не вечное ожидание
	lib = Load(t.TempDir(), "sounds", defs, nil, false)
	if g = lib.Get("gank"); g.Pending || g.Err == "" || len(g.Clips) != 1 {
		t.Fatalf("без TTS: %+v", g)
	}
}
