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
