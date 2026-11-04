package app

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/timers"
)

// Модуль Dota 2: голосовые напоминания по игровому времени (руны, стаки…),
// Рошан, кнопки-оповещения команде. Пока модуль выключен — ничего из этого не загружается.

const (
	reminderPrefix = "dota_rem_"
	roshPrefix     = "dota_rosh_"
	defaultSignal  = "builtin:chime"
)

func reminderSound(id string) string { return reminderPrefix + id }
func roshSound(kind string) string   { return roshPrefix + kind }

// buildDefs — звуки для библиотеки: из конфига (Dota — только при включённом модуле)
// и служебные звуки напоминаний.
func buildDefs(cfg *config.Config) map[string]sounds.Def {
	defs := map[string]sounds.Def{}
	for id, d := range cfg.Sounds {
		if d.Dota && !cfg.Dota.Enabled {
			continue
		}
		defs[id] = d
	}
	if !cfg.Dota.Enabled {
		return defs
	}
	for _, r := range cfg.Dota.Reminders {
		if r.Enabled {
			defs[reminderSound(r.ID)] = reminderDef(cfg, r)
		}
	}
	if cfg.Dota.Rosh {
		bus := cfg.Dota.VoiceTo
		vol := cfg.Dota.Volume
		mk := func(say string) sounds.Def {
			return sounds.Def{Files: []string{defaultSignal, "tts:" + say}, Seq: true, Bus: bus, Volume: vol,
				Mode: sounds.ModeInterrupt, Dota: true, Hidden: true}
		}
		defs[roshSound("aegis")] = mk(withIn(i18n.T("Аегис пропадёт"), cfg.Rosh.AegisWarn))
		defs[roshSound("min")] = mk(withIn(i18n.T("Рошан может появиться"), cfg.Rosh.MinWarn))
		defs[roshSound("max")] = mk(i18n.T("Рошан точно возродился"))
	}
	return defs
}

// reminderDef — сигнал + фраза («Руна мудрости через 20 секунд»).
func reminderDef(cfg *config.Config, r config.Reminder) sounds.Def {
	var files []string
	files = append(files, signalFiles(cfg, r.Sound)...)
	if say := ReminderPhrase(r); say != "" {
		files = append(files, "tts:"+say)
	}
	bus := r.To
	if bus == "" {
		bus = cfg.Dota.VoiceTo
	}
	return sounds.Def{Files: files, Seq: true, Bus: bus, Volume: cfg.Dota.Volume, Mode: sounds.ModeInterrupt,
		Dota: true, Hidden: true, Label: r.Label}
}

// signalFiles — звук перед фразой: "" — колокольчик, "none" — без сигнала, builtin:x, путь или id звука.
func signalFiles(cfg *config.Config, ref string) []string {
	switch {
	case ref == "":
		return []string{defaultSignal}
	case ref == "none":
		return nil
	case strings.HasPrefix(ref, "builtin:"), strings.Contains(ref, "/"), strings.Contains(ref, "\\"):
		return []string{ref}
	}
	if d, ok := cfg.Sounds[ref]; ok && len(d.Files) > 0 {
		if d.Seq {
			return d.Files
		}
		return d.Files[:1]
	}
	if _, ok := sounds.Builtin(ref); ok {
		return []string{"builtin:" + ref}
	}
	if cfg.SoundsDir != "" {
		return []string{cfg.SoundsDir + "/" + ref + ".mp3"}
	}
	return []string{defaultSignal}
}

// ReminderPhrase — что сказать: своя фраза или «<название> через N секунд».
// В своей фразе {время} (или {time}) заменяется на «через 40 секунд» (или «сейчас», если «До» = 0).
// Фраза по-английски — и время по-английски: «in 40 seconds», «now».
func ReminderPhrase(r config.Reminder) string {
	if s := strings.TrimSpace(r.Say); s != "" {
		if s == "-" {
			return "" // только сигнал
		}
		if strings.Contains(s, "{время}") || strings.Contains(s, "{time}") {
			in := inTime(r.Before, phraseEn(strings.NewReplacer("{время}", "", "{time}", "").Replace(s)))
			s = strings.NewReplacer("{время}", in, "{time}", in).Replace(s)
		}
		return s
	}
	return withIn(r.Label, r.Before)
}

func withIn(label string, sec int) string {
	if sec <= 0 {
		return label
	}
	return label + " " + inTime(sec, phraseEn(label))
}

// phraseEn — фраза на английском: латиница без русских букв (без букв вовсе — как язык интерфейса).
func phraseEn(s string) bool {
	if i18n.Cyrillic(s) {
		return false
	}
	for _, r := range s {
		if r < 128 && unicode.IsLetter(r) {
			return true
		}
	}
	return i18n.En()
}

// inTime — «через 40 секунд» или «in 40 seconds»; 0 — «сейчас» / «now».
func inTime(sec int, en bool) string {
	switch {
	case sec <= 0 && en:
		return "now"
	case sec <= 0:
		return "сейчас"
	case en:
		return "in " + SecondsEn(sec)
	}
	return "через " + Seconds(sec)
}

// SecondsEn — «20 seconds», «1 minute», «1 minute 30 seconds».
func SecondsEn(n int) string {
	unit := func(n int, w string) string {
		if n == 1 {
			return "1 " + w
		}
		return fmt.Sprintf("%d %ss", n, w)
	}
	if n >= 60 {
		out := unit(n/60, "minute")
		if s := n % 60; s > 0 {
			out += " " + unit(s, "second")
		}
		return out
	}
	return unit(n, "second")
}

// Seconds — «20 секунд», «1 минуту», «1 минуту 30 секунд».
func Seconds(n int) string {
	if n >= 60 {
		m, s := n/60, n%60
		out := plural(m, "минуту", "минуты", "минут")
		if m == 1 {
			out = "минуту"
		}
		if s > 0 {
			out += " " + plural(s, "секунду", "секунды", "секунд")
		}
		return out
	}
	return plural(n, "секунду", "секунды", "секунд")
}

func plural(n int, one, few, many string) string {
	w := many
	switch m10, m100 := n%10, n%100; {
	case m10 == 1 && m100 != 11:
		w = one
	case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
		w = few
	}
	return fmt.Sprintf("%d %s", n, w)
}

// buildTimers — напоминания модуля Dota в виде таймеров (и подписи для «Ближайшие»).
func buildTimers(cfg *config.Config) ([]timers.Timer, map[string]string) {
	labels := map[string]string{"aegis_expire": "Аегис пропадёт", "rosh_min": "Рошан может появиться", "rosh_max": "Рошан точно возродился"}
	if !cfg.Dota.Enabled {
		return nil, labels
	}
	var out []timers.Timer
	for _, r := range cfg.Dota.Reminders {
		labels[r.ID] = r.Label
		out = append(out, timers.Timer{ID: r.ID, Label: r.Label, Sound: reminderSound(r.ID), At: r.At, Start: r.Start,
			Every: r.Every, Until: r.Until, Skip: r.Skip, WarnBefore: r.Before, Disabled: !r.Enabled})
	}
	return out, labels
}

// playReminder играет напоминание; если другое ещё звучит — ждёт, чтобы фразы не накладывались.
func (a *App) playReminder(id string) {
	a.mu.Lock()
	lib := a.lib
	a.mu.Unlock()
	s := lib.Get(id)
	if s == nil {
		a.Play(id) // старые таймеры и звуки пользователя
		return
	}
	a.remMu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if a.remUntil.After(now) {
		wait = a.remUntil.Sub(now)
	}
	a.remUntil = now.Add(wait + time.Duration((s.Duration()+0.35)*float64(time.Second)))
	a.remMu.Unlock()
	play := func() {
		a.Play(id)
		if s.Label != "" {
			a.toast(Toast{Emoji: "⏰", Text: s.Label, Sub: "напоминание"})
		}
	}
	if wait > 0 {
		time.AfterFunc(wait, play)
	} else {
		play()
	}
}

// TestReminder проигрывает напоминание прямо сейчас (кнопка ▶ в интерфейсе), в том числе выключенное.
func (a *App) TestReminder(r config.Reminder) error {
	cfg := a.Config()
	d := reminderDef(cfg, r)
	var clips []sounds.Clip
	for _, f := range d.Files {
		if text, ok := strings.CutPrefix(f, "tts:"); ok {
			a.mu.Lock()
			sp := a.speech
			a.mu.Unlock()
			if sp == nil {
				continue
			}
			c, err := sp.Say(text)
			if err != nil {
				return err
			}
			clips = append(clips, sounds.Normalize(c))
			continue
		}
		lib := sounds.Load(a.BaseDir, cfg.SoundsDir, map[string]sounds.Def{"x": {Files: []string{f}}}, nil, cfg.Normalize)
		if s := lib.Get("x"); s != nil && len(s.Clips) > 0 {
			clips = append(clips, s.Clips[0])
		}
	}
	if len(clips) == 0 {
		return fmt.Errorf("нечего играть")
	}
	vol := float32(cfg.Dota.Volume)
	return a.playClip("preview", sounds.Concat(clips, sounds.SeqGap), vol, sounds.ModeInterrupt, sounds.ParseBus(d.Bus))
}

// applyGSI запускает приём данных от Dota 2, только когда модуль включён.
func (a *App) applyGSI(cfg *config.Config) {
	want := ""
	if cfg.Dota.Enabled && cfg.GSI.Enabled && cfg.GSI.Addr != "" {
		want = cfg.GSI.Addr + "|" + cfg.GSI.Token
	}
	a.gsiMu.Lock()
	defer a.gsiMu.Unlock()
	if want == a.gsiAddr {
		return
	}
	if a.gsiSrv != nil {
		a.gsiSrv.Close()
		a.gsiSrv = nil
		log.Print("GSI: приём данных от Dota 2 остановлен")
	}
	a.gsiAddr = want
	if want == "" {
		return
	}
	srv := &gsi.Server{Addr: cfg.GSI.Addr, Token: cfg.GSI.Token, OnState: a.OnGSI}
	if err := srv.Start(); err != nil {
		log.Printf("GSI: порт %s занят: %v", cfg.GSI.Addr, err)
		a.gsiAddr = ""
		return
	}
	a.gsiSrv = srv
	log.Printf("GSI: жду Dota 2 на http://%s/", cfg.GSI.Addr)
}

// StopGSI — при выходе.
func (a *App) StopGSI() {
	a.gsiMu.Lock()
	defer a.gsiMu.Unlock()
	if a.gsiSrv != nil {
		a.gsiSrv.Close()
		a.gsiSrv = nil
	}
	a.gsiAddr = ""
}
