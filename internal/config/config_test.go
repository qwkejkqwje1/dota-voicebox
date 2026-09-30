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
	if err != nil || c.Hotkeys["Num1"] != "sound:siren" || len(c.Timers) != 5 {
		t.Fatalf("%v %+v", err, c)
	}
	os.WriteFile(p, []byte(`{"ptt":{"key":"B","auto":true},"hotkeys":{"F9":"stop"}}`), 0o644)
	c, err = Load(p)
	if err != nil || c.PTT.Key != "B" || len(c.Hotkeys) != 1 || len(c.Timers) != 5 {
		t.Fatalf("%v %+v", err, c.Hotkeys)
	}
}
