package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
)

// Конфиг по умолчанию для английского интерфейса: все строки переведены, разметка и ключи — те же.
func TestDefaultForEnglish(t *testing.T) {
	en := DefaultFor("en")
	if i18n.Cyrillic(string(en)) {
		var left []string
		jsonStrRe.ReplaceAllFunc(en, func(q []byte) []byte {
			if i18n.Cyrillic(string(q)) {
				left = append(left, string(q))
			}
			return q
		})
		t.Fatalf("не переведено: %q", left)
	}
	var ru, eng map[string]json.RawMessage
	if err := json.Unmarshal(DefaultJSON, &ru); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(en, &eng); err != nil {
		t.Fatal(err)
	}
	if len(ru) != len(eng) {
		t.Fatalf("ключей: %d → %d", len(ru), len(eng))
	}
	p := filepath.Join(t.TempDir(), "config.json")
	if created, err := EnsureFile(p, "en"); err != nil || !created {
		t.Fatal(created, err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.UILang != "en" || c.Sounds["airhorn"].Label != "Party horn" {
		t.Fatalf("ui_lang=%q airhorn=%q", c.UILang, c.Sounds["airhorn"].Label)
	}
	b, _ := os.ReadFile(p)
	if i18n.Cyrillic(string(b)) {
		t.Fatal("в английском config.json русские строки")
	}
}
