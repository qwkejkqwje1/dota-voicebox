//go:build !windows

package winapi

import "testing"

func TestUILanguage(t *testing.T) {
	for _, c := range []struct{ all, msg, lang, want string }{
		{"", "", "en_US.UTF-8", "en-US"},
		{"", "", "ru_RU.UTF-8", "ru-RU"},
		{"", "", "C.UTF-8", ""}, // сборочные серверы: языка нет
		{"", "", "POSIX", ""},
		{"C", "", "de_DE", "de-DE"},
		{"uk_UA.UTF-8", "", "en_US.UTF-8", "uk-UA"},
		{"", "", "", ""},
	} {
		t.Setenv("LC_ALL", c.all)
		t.Setenv("LC_MESSAGES", c.msg)
		t.Setenv("LANG", c.lang)
		if got := UILanguage(); got != c.want {
			t.Errorf("LC_ALL=%q LANG=%q: %q, want %q", c.all, c.lang, got, c.want)
		}
	}
}
