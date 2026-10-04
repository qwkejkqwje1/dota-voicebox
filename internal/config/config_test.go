package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAndOverride(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	if created, err := EnsureFile(p); err != nil || !created {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil || c.Hotkeys["Num2"] != "sound:wisdom_rune@both" || !c.Scripts["gank_mid"] || len(c.Timers) != 5 {
		t.Fatalf("%v %+v", err, c)
	}
	os.WriteFile(p, []byte(`{"ptt":{"key":"B","auto":true},"hotkeys":{"F9":"stop"}}`), 0o644)
	c, err = Load(p)
	if err != nil || c.PTT.Key != "B" || len(c.Hotkeys) != 1 || len(c.Timers) != 5 {
		t.Fatalf("%v %+v", err, c.Hotkeys)
	}
}

func TestMigrateV1AndBackups(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, []byte(`{"mic_gain": 1.7, "hotkeys": {"F9": "stop"}}`), 0o644)
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != Version || c.MicGain != 1.7 || c.Hotkeys["F9"] != "stop" || !c.Update.AutoCheck {
		t.Fatalf("%+v", c)
	}
	for i := 0; i < 8; i++ {
		c.MicGain = float64(i)
		if err := Save(p, c); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(Backups(p)); n != 5 {
		t.Fatalf("копий: %d", n)
	}
	if err := Restore(p, Backups(p)[0]); err != nil {
		t.Fatal(err)
	}
	c2, _ := Load(p)
	if c2.MicGain != 6 {
		t.Fatalf("restore: %v", c2.MicGain)
	}
}
