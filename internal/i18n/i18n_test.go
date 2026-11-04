package i18n

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestT(t *testing.T) {
	load()
	defer use(Dict())
	use(map[string]string{
		"Звуки":                    "Sounds",
		"Звуков:":                  "Sounds:",
		"звук %q не найден":        "sound %q not found",
		"не удалось сохранить: %v": "could not save: %v",
		"нет места на диске":       "no space left on device",
		"%s — %d из %d":            "%s — %d of %d",
		"%[2]d минут на %[1]s":     "%[1]s for %[2]d minutes",
		"Громкость %d%%":           "Volume %d%%",
		"%s: %s":                   "%s: %s",
		"Голос читалки: %s":        "Reader voice: %s",
		"Ехидный":                  "Sly",
		"секунда":                  "second",
		"секунд":                   "seconds",
	})
	SetLang("ru")
	if T("Звуки") != "Звуки" {
		t.Fatal("русский интерфейс — без перевода")
	}
	SetLang("en")
	defer SetLang("ru")
	for in, want := range map[string]string{
		"Звуки":                 "Sounds",
		"  Звуков: ":            "  Sounds: ",
		`звук "гонг" не найден`: `sound "гонг" not found`,
		"не удалось сохранить: нет места на диске": "could not save: no space left on device",
		"Голос читалки: Ехидный":                   "Reader voice: Sly",
		"Громкость 50%":                            "Volume 50%",
		"мид — 2 из 3":                             "мид — 2 of 3",
		"5 минут на мид":                           "мид for 5 minutes",
		"а: б":                                     "а: б", // шаблон без русского текста не используется
		"Ехидный · Звуки":                          "Sly · Sounds",
		"Ехидный\nнечто":                           "Sly\nнечто",
		"Неизвестно":                               "Неизвестно",
		"OK":                                       "OK",
	} {
		if got := T(in); got != want {
			t.Errorf("T(%q) = %q, want %q", in, got, want)
		}
	}
	if Plural(1, "секунда", "секунды", "секунд") != "second" || Plural(3, "секунда", "секунды", "секунд") != "seconds" {
		t.Fatal("Plural en")
	}
	SetLang("ru")
	if Plural(3, "секунда", "секунды", "секунд") != "секунды" || Plural(11, "секунда", "секунды", "секунд") != "секунд" ||
		Plural(21, "секунда", "секунды", "секунд") != "секунда" {
		t.Fatal("Plural ru")
	}
	SetLang("en")
	b, _ := JSON(map[string]any{"title": "Звуки", "n": 1.5, "config": map[string]any{"label": "Звуки"},
		"list": []any{"Ехидный", map[string]any{"mic": "Звуки", "error": `звук "x" не найден`}}})
	var back map[string]any
	json.Unmarshal(b, &back)
	if s := string(b); !strings.Contains(s, `"title":"Sounds"`) || !strings.Contains(s, `"config":{"label":"Звуки"}`) ||
		!strings.Contains(s, `"Sly"`) || !strings.Contains(s, `"mic":"Звуки"`) || !strings.Contains(s, `sound \"x\" not found`) || back["n"] != 1.5 {
		t.Fatalf("JSON: %s", s)
	}
	// порядок ключей и непереведённое — как было; журнал — без времени в начале; звуки пользователя — нет
	in := `{"z":"Звуки","a":{"logs":["12:00:01 Звуки","Звуки"],"sounds":[{"title":"Звуки"}]}, "n":1e3,"b":[1, 2]}`
	want := `{"z":"Sounds","a":{"logs":["12:00:01 Sounds","Sounds"],"sounds":[{"title":"Звуки"}]},"n":1e3,"b":[1, 2]}`
	if got := string(TranslateJSON([]byte(in))); got != want {
		t.Fatalf("TranslateJSON:\n%s\n%s", got, want)
	}
	if Line("09:15:00 Звуки") != "09:15:00 Sounds" {
		t.Fatal("Line")
	}
}

func TestDetect(t *testing.T) {
	for in, want := range map[string]string{"ru-RU": "ru", "uk-UA": "ru", "en-US": "en", "de-DE": "en", "": "ru", "kk": "ru", "be_BY": "ru"} {
		if Detect(in) != want {
			t.Errorf("Detect(%q)", in)
		}
	}
}

// Словарь en.json: валидный JSON, у шаблонов в переводе столько же подстановок, сколько в оригинале.
func TestDictionary(t *testing.T) {
	d := map[string]string{}
	if err := json.Unmarshal(enJSON, &d); err != nil {
		t.Fatal(err)
	}
	for k, v := range d {
		if strings.TrimSpace(v) == "" {
			t.Errorf("пустой перевод: %q", k)
			continue
		}
		if Cyrillic(v) {
			t.Errorf("в переводе русские буквы: %q → %q", k, v)
		}
		nk, nv := len(verbRe.FindAllString(k, -1)), len(verbRe.FindAllString(v, -1))
		if nk != nv {
			t.Errorf("подстановки: %q (%d) → %q (%d)", k, nk, v, nv)
		}
	}
}
