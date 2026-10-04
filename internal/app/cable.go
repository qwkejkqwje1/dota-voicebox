package app

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// Проверки и автонастройка виртуального кабеля на уровне Windows:
// кабель не должен быть устройством по умолчанию, частота — 48 кГц.

const cableRate = 48000

var prevDefaultOut string // устройство по умолчанию до установки кабеля

// isVirtual: похоже на кабель или выбрано пользователем как кабель / другой конец кабеля.
func (a *App) isVirtual(e winapi.AudioEndpoint) bool {
	c := a.Config()
	for _, n := range []string{c.Devices.VoiceOut, c.Devices.CableRec} {
		if n != "" && (strings.EqualFold(e.Name, n) || strings.HasPrefix(strings.ToLower(e.Name), strings.ToLower(n)+" (")) {
			return true
		}
	}
	return audio.IsVirtual(e.Name)
}

func shortName(n string) string {
	if i := strings.Index(n, " ("); i > 0 {
		return n[:i]
	}
	return n
}

// cableChecks добавляет пункты «кабель не по умолчанию» и «частота кабеля».
func (a *App) cableChecks(add func(Check)) {
	eps, err := winapi.AudioEndpoints()
	if err != nil || len(eps) == 0 {
		return
	}
	ch := Check{ID: "cable_default", Title: "Звуки Windows не идут в эфир", OK: true}
	for _, e := range eps {
		if e.Capture || !(e.Default || e.DefaultComm) {
			continue
		}
		if a.isVirtual(e) {
			ch.OK = false
			ch.Detail = fmt.Sprintf("«%s» стоит устройством вывода по умолчанию — звуки Windows, браузера и музыки польются в эфир", shortName(e.Name))
			ch.Fix, ch.FixLbl = "default_out", "Вернуть наушники"
			break
		}
		ch.Detail = "по умолчанию: " + shortName(e.Name)
	}
	add(ch)

	var bad []string
	found := false
	for _, e := range eps {
		if !a.isVirtual(e) {
			continue
		}
		found = true
		if e.Rate != 0 && e.Rate != cableRate {
			bad = append(bad, fmt.Sprintf("%s — %d Гц", shortName(e.Name), e.Rate))
		}
	}
	if !found {
		return
	}
	ch = Check{ID: "cable_rate", Title: "Частота кабеля 48 кГц", OK: len(bad) == 0, Detail: "48000 Гц ✓"}
	if !ch.OK {
		ch.Detail = strings.Join(bad, ", ") + ": лишнее преобразование частоты — задержка и потеря качества"
		ch.Fix, ch.FixLbl = "cable_rate", "Поставить 48 кГц"
		ch.Manual = "Если не получится: Панель звука → CABLE Input → Свойства → Дополнительно → 48000 Гц; то же для CABLE Output на вкладке «Запись»."
	}
	add(ch)
}

// fixDefaultOut возвращает устройством по умолчанию наушники/колонки.
func (a *App) fixDefaultOut() error {
	eps, err := winapi.AudioEndpoints()
	if err != nil {
		return err
	}
	want := a.Config().Devices.Monitor
	if want == "" {
		want = a.out.Status().Monitor
	}
	var target *winapi.AudioEndpoint
	score := -1
	for i := range eps {
		e := &eps[i]
		if e.Capture || a.isVirtual(*e) {
			continue
		}
		sc := 0
		if e.ID == prevDefaultOut {
			sc = 2
		} else if want != "" && e.Name == want {
			sc = 1
		}
		if sc > score {
			target, score = e, sc
		}
	}
	if target == nil {
		return errors.New("не нашёл наушников или колонок — подключите их")
	}
	if err := winapi.SetDefaultAudio(target.ID); err != nil {
		winapi.OpenSoundPanel(0)
		return fmt.Errorf("не получилось (%v) — открыл «Панель звука»: выберите наушники → «По умолчанию»", err)
	}
	log.Printf("Устройство по умолчанию: %s", target.Name)
	return nil
}

// fixCableRate ставит 48 кГц на все виртуальные устройства.
func (a *App) fixCableRate() error {
	eps, err := winapi.AudioEndpoints()
	if err != nil {
		return err
	}
	var errs []string
	changed := false
	for _, e := range eps {
		if !a.isVirtual(e) || e.Rate == 0 || e.Rate == cableRate {
			continue
		}
		if err := winapi.SetEndpointRate(e.ID, cableRate); err != nil {
			errs = append(errs, shortName(e.Name))
			continue
		}
		changed = true
		log.Printf("%s: частота %d → %d Гц", e.Name, e.Rate, cableRate)
	}
	if changed {
		a.Fix("audio")
	}
	if len(errs) > 0 {
		tab := 0
		if strings.Contains(strings.Join(errs, ""), "Output") {
			tab = 1
		}
		winapi.OpenSoundPanel(tab)
		return fmt.Errorf("Windows не дала изменить частоту (%s) — открыл «Панель звука», поставьте 48000 Гц в «Свойства → Дополнительно»", strings.Join(errs, ", "))
	}
	return nil
}

// rememberDefaultOut запоминает текущее устройство по умолчанию (перед установкой кабеля).
func (a *App) rememberDefaultOut() {
	eps, _ := winapi.AudioEndpoints()
	for _, e := range eps {
		if !e.Capture && e.Default && !a.isVirtual(e) {
			prevDefaultOut = e.ID
		}
	}
}

// afterCableInstall: установщик VB-Cable часто делает кабель устройством по умолчанию — возвращаем как было.
func (a *App) afterCableInstall() {
	var cableDefault bool
	eps, _ := winapi.AudioEndpoints()
	for _, e := range eps {
		if !e.Capture && (e.Default || e.DefaultComm) && a.isVirtual(e) {
			cableDefault = true
		}
	}
	if cableDefault {
		if err := a.fixDefaultOut(); err != nil {
			log.Printf("⚠ %v", err)
		} else {
			log.Print("Кабель был назначен устройством по умолчанию — вернул ваши наушники ✓")
		}
	}
	if err := a.fixCableRate(); err != nil {
		log.Printf("⚠ %v", err)
	}
}

// watchDefault — сторож: кабель стал устройством вывода по умолчанию → уведомление.
var lastDefaultWarn string

func (a *App) watchDefault() {
	eps, err := winapi.AudioEndpoints()
	if err != nil {
		return
	}
	cur := ""
	for _, e := range eps {
		if !e.Capture && e.Default && a.isVirtual(e) {
			cur = e.Name
		}
	}
	if cur != "" && cur != lastDefaultWarn {
		log.Printf("🔔 «%s» стал устройством по умолчанию — звуки Windows пойдут в эфир. «Настройка → Вернуть наушники»", shortName(cur))
	}
	lastDefaultWarn = cur
}
