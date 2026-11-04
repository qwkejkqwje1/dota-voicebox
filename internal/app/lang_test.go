package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

// Язык в мастере первого запуска: пока ничего не настроено, звуки, фразы и напоминания по умолчанию
// заменяются на новый язык (устройства остаются), примеры скриптов — тоже. После настройки — только язык.
func TestSetUILang(t *testing.T) {
	a, _ := newApp(t, false)
	t.Cleanup(func() { i18n.SetLang("ru") })
	if err := a.UpdateConfig(func(c *config.Config) { c.UILang = "ru"; c.Devices.Mic = "Mic X" }); err != nil {
		t.Fatal(err)
	}
	if err := a.SetUILang("en"); err != nil {
		t.Fatal(err)
	}
	c := a.Config()
	if c.UILang != "en" || c.Devices.Mic != "Mic X" || c.Sounds["airhorn"].Label != "Party horn" || !i18n.En() {
		t.Fatalf("ui_lang=%q mic=%q airhorn=%q en=%v", c.UILang, c.Devices.Mic, c.Sounds["airhorn"].Label, i18n.En())
	}
	if b, _ := os.ReadFile(filepath.Join(a.BaseDir, "scripts", "chat_reader.js")); len(b) == 0 || i18n.Cyrillic(string(b)) {
		t.Errorf("пример скрипта не переведён: %.60q", b)
	}
	// после мастера звуки пользователя не трогаем — меняется только язык
	if err := a.UpdateConfig(func(c *config.Config) { c.SetupDone = true }); err != nil {
		t.Fatal(err)
	}
	if err := a.SetUILang("ru"); err != nil {
		t.Fatal(err)
	}
	if c = a.Config(); c.UILang != "ru" || c.Sounds["airhorn"].Label != "Party horn" || i18n.En() {
		t.Fatalf("ui_lang=%q airhorn=%q en=%v", c.UILang, c.Sounds["airhorn"].Label, i18n.En())
	}
	if err := a.SetUILang("de"); err == nil {
		t.Fatal("неизвестный язык должен быть ошибкой")
	}
}

// Встроенные звуки без описания в конфиге подписаны на языке интерфейса; добавленные на панель
// в английском интерфейсе получают английскую подпись.
func TestBuiltinLabelEnglish(t *testing.T) {
	t.Cleanup(func() { i18n.SetLang("ru") })
	s := &sounds.Sound{ID: "alert", Label: "Короткая сирена", Source: "builtin"}
	if got := DisplayLabel(s); got != "Короткая сирена" {
		t.Fatalf("ru: %q", got)
	}
	i18n.SetLang("en")
	if got := DisplayLabel(s); got != "Short siren" {
		t.Fatalf("en: %q", got)
	}
	if got := DisplayLabel(&sounds.Sound{ID: "x", Label: "Мой звук", Source: "config"}); got != "Мой звук" {
		t.Fatalf("подпись пользователя не переводится: %q", got)
	}
	a, _ := newApp(t, false)
	i18n.SetLang("en") // новое приложение берёт язык из конфига
	id, err := a.AddBuiltin("alert")
	if err != nil {
		t.Fatal(err)
	}
	if l := a.Config().Sounds[id].Label; l != "Short siren" {
		t.Fatalf("added builtin label %q", l)
	}
}
