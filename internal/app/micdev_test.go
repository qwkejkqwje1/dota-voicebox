package app

import (
	"testing"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
)

func TestNextMic(t *testing.T) {
	capture := []audio.Device{
		{Name: "Микрофон (Realtek(R) Audio)", IsDefault: true},
		{Name: "CABLE Output (VB-Audio Virtual Cable)", Virtual: true},
		{Name: "Микрофон (HyperX Cloud II)"},
		{Name: "Voicemeeter Out B1 (VB-Audio Voicemeeter VAIO)", Virtual: true},
	}
	list := micChoices(capture)
	if len(list) != 2 {
		t.Fatalf("micChoices = %v", list)
	}
	if got := nextMic(list, "Микрофон (Realtek(R) Audio)"); got != "Микрофон (HyperX Cloud II)" {
		t.Errorf("next after Realtek = %q", got)
	}
	if got := nextMic(list, "микрофон (hyperx cloud ii)"); got != "Микрофон (Realtek(R) Audio)" {
		t.Errorf("по кругу = %q", got)
	}
	if got := nextMic(list, "по умолчанию"); got != list[0] {
		t.Errorf("неизвестный текущий = %q", got)
	}
	if nextMic(nil, "x") != "" {
		t.Error("пустой список")
	}
}

func TestMissingBack(t *testing.T) {
	capture := []audio.Device{{Name: "Микрофон (Realtek(R) Audio)"}, {Name: "Микрофон (HyperX Cloud II)"}}
	playback := []audio.Device{{Name: "Динамики (Realtek(R) Audio)"}}
	if missingBack(audio.Status{}, capture, playback) {
		t.Error("ничего не пропадало")
	}
	if !missingBack(audio.Status{MicMissing: "Микрофон (HyperX Cloud II)"}, capture, playback) {
		t.Error("микрофон вернулся")
	}
	if missingBack(audio.Status{MicMissing: "USB PnP Sound Device"}, capture, playback) {
		t.Error("микрофона всё ещё нет")
	}
	if !missingBack(audio.Status{MonitorMissing: "Динамики"}, capture, playback) {
		t.Error("наушники по части имени")
	}
}
