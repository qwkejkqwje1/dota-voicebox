package app

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// Check — один пункт мастера настройки.
type Check struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"` // действие, которое исправит пункт (кнопка в интерфейсе)
	FixLbl string `json:"fix_label,omitempty"`
	Manual string `json:"manual,omitempty"` // что сделать руками, если автоматически нельзя
}

type SetupStatus struct {
	Checks    []Check `json:"checks"`
	AllOK     bool    `json:"all_ok"`
	Autostart bool    `json:"autostart"`
	Busy      string  `json:"busy,omitempty"`
}

var setupMu sync.Mutex
var setupBusy string

func (a *App) gsiURI(c *config.Config) string { return "http://" + c.GSI.Addr + "/" }

// SetupStatus проверяет всё, что нужно для работы.
func (a *App) SetupStatus() SetupStatus {
	c := a.Config()
	var st SetupStatus
	add := func(ch Check) { st.Checks = append(st.Checks, ch) }

	_, playback, derr := audio.Devices()
	cable := audio.FindCable(playback)
	ch := Check{ID: "cable", Title: "Виртуальный кабель (VB-Audio Cable)"}
	switch {
	case derr != nil:
		ch.Detail = "не удалось получить список устройств: " + derr.Error()
	case cable != "":
		ch.OK, ch.Detail = true, cable
	default:
		ch.Detail = "не установлен — без него звуки не попадут в игру"
		ch.Fix, ch.FixLbl = "cable", "Скачать и установить"
	}
	add(ch)

	as := a.out.Status()
	ch = Check{ID: "audio", Title: "Аудио", OK: as.Running}
	if as.Running {
		ch.Detail = fmt.Sprintf("🎤 %s → %s · 🎧 %s", as.Mic, as.VoiceOut, as.Monitor)
	} else {
		ch.Detail = as.Error
		ch.Fix, ch.FixLbl = "audio", "Перезапустить аудио"
	}
	add(ch)

	add(Check{ID: "dota_input", Title: "Микрофон в Dota 2", OK: as.Running,
		Detail: "В Dota 2 → Настройки → Звук → Устройство ввода выберите «CABLE Output»",
		Manual: "Игра не даёт выбрать устройство программно — это единственный ручной шаг (один раз)."})

	root := winapi.SteamRoot()
	dota, derr2 := winapi.FindDota(root)
	ch = Check{ID: "dota", Title: "Dota 2", OK: derr2 == nil, Detail: dota}
	if derr2 != nil {
		ch.Detail = derr2.Error()
	}
	add(ch)

	if derr2 == nil {
		ok := winapi.GSIInstalled(dota, a.gsiURI(c), c.GSI.Token)
		ch = Check{ID: "gsi", Title: "Game State Integration (таймеры)", OK: ok}
		if ok {
			ch.Detail = "установлен"
		} else {
			ch.Detail = "не установлен"
			ch.Fix, ch.FixLbl = "gsi", "Установить"
		}
		add(ch)

		has, lerr := winapi.HasLaunchOption(root)
		ch = Check{ID: "launch", Title: "Параметр запуска -gamestateintegration", OK: has}
		switch {
		case lerr != nil:
			ch.Detail = lerr.Error()
			ch.Manual = "Steam → Dota 2 → Свойства → Параметры запуска: -gamestateintegration"
		case has:
			ch.Detail = "добавлен"
		default:
			ch.Detail = "не добавлен"
			ch.Fix, ch.FixLbl = "launch", "Добавить"
			if winapi.SteamRunning() {
				ch.Detail += " (Steam будет перезапущен)"
			}
		}
		add(ch)

		key, file, kerr := winapi.FindVoiceKey(root, dota)
		ch = Check{ID: "ptt", Title: "Кнопка голосового чата"}
		switch {
		case c.PTT.Key != "" && (kerr != nil || key == c.PTT.Key):
			ch.OK, ch.Detail = true, c.PTT.Key
			if kerr == nil {
				ch.Detail += " (совпадает с биндом Dota 2)"
			}
		case kerr == nil:
			ch.Detail = fmt.Sprintf("в Dota 2 назначена %s, в программе — %q", key, c.PTT.Key)
			ch.Fix, ch.FixLbl = "ptt", "Взять из Dota 2"
		default:
			ch.Detail = kerr.Error()
			ch.Manual = "Укажите клавишу вручную в разделе «Настройка → Голосовой чат»"
		}
		_ = file
		add(ch)
	}

	a.gsiMu.Lock()
	conn := !a.lastGSI.IsZero() && time.Since(a.lastGSI) < 40*time.Second
	a.gsiMu.Unlock()
	ch = Check{ID: "gsi_live", Title: "Связь с игрой", OK: conn}
	if conn {
		ch.Detail = "Dota 2 передаёт данные"
	} else if winapi.DotaRunning() {
		ch.Detail = "Dota 2 запущена, но данных нет — перезапустите игру после установки GSI"
	} else {
		ch.Detail = "появится, когда вы запустите Dota 2"
	}
	add(ch)

	st.AllOK = true
	for _, c := range st.Checks {
		if !c.OK && c.ID != "gsi_live" {
			st.AllOK = false
		}
	}
	st.Autostart = winapi.Autostart()
	setupMu.Lock()
	st.Busy = setupBusy
	setupMu.Unlock()
	return st
}

func busy(what string) func() {
	setupMu.Lock()
	setupBusy = what
	setupMu.Unlock()
	return func() {
		setupMu.Lock()
		setupBusy = ""
		setupMu.Unlock()
	}
}

// Fix выполняет автоматическое исправление пункта.
func (a *App) Fix(id string) error {
	root := winapi.SteamRoot()
	switch id {
	case "audio":
		a.mu.Lock()
		a.devOpts = nil
		a.mu.Unlock()
		return a.Reload()

	case "gsi":
		dota, err := winapi.FindDota(root)
		if err != nil {
			return err
		}
		c := a.Config()
		p, err := winapi.InstallGSI(dota, a.gsiURI(c), c.GSI.Token)
		if err == nil {
			log.Printf("GSI установлен: %s", p)
		}
		return err

	case "launch":
		defer busy("Добавляю параметр запуска в Steam…")()
		wasRunning := winapi.SteamRunning()
		if winapi.DotaRunning() {
			return errors.New("сначала закройте Dota 2")
		}
		if wasRunning {
			log.Print("Закрываю Steam, чтобы изменить параметры запуска…")
			if err := winapi.ShutdownSteam(root); err != nil {
				return err
			}
		}
		n, err := winapi.AddLaunchOption(root)
		if err != nil {
			return err
		}
		log.Printf("Параметр -gamestateintegration добавлен (%d профиль(ей) Steam)", n)
		if wasRunning {
			winapi.StartSteam(root)
		}
		return nil

	case "ptt":
		dota, _ := winapi.FindDota(root)
		key, _, err := winapi.FindVoiceKey(root, dota)
		if err != nil {
			return err
		}
		log.Printf("Кнопка голосового чата из Dota 2: %s", key)
		return a.UpdateConfig(func(c *config.Config) { c.PTT.Key = key })

	case "cable":
		done := busy("Скачиваю VB-Audio Virtual Cable…")
		setup, err := winapi.DownloadVBCable(filepath.Join(os.TempDir(), "voicebox_vbcable"))
		done()
		if err != nil {
			winapi.OpenURL("https://vb-audio.com/Cable/")
			return fmt.Errorf("не удалось скачать (%v) — открыл сайт, установите вручную", err)
		}
		log.Print("Запускаю установщик VB-Cable (подтвердите запрос администратора)…")
		if err := winapi.RunElevated(setup, "-i -h"); err != nil {
			return err
		}
		go a.waitForCable()
		return nil
	}
	return fmt.Errorf("неизвестное действие %q", id)
}

// waitForCable ждёт появления кабеля после установки и перезапускает аудио.
func (a *App) waitForCable() {
	defer busy("Жду завершения установки VB-Cable…")()
	for i := 0; i < 90; i++ {
		time.Sleep(2 * time.Second)
		_, p, err := audio.Devices()
		if err == nil && audio.FindCable(p) != "" {
			log.Print("VB-Cable установлен ✓ — запускаю аудио")
			a.Fix("audio")
			return
		}
	}
	log.Print("Кабель не появился. Возможно, нужна перезагрузка Windows.")
}

// AutoSetup — всё, что можно сделать без участия пользователя. Вызывается при первом запуске.
func (a *App) AutoSetup(allowSteamRestart bool) []string {
	defer busy("Автонастройка…")()
	var report []string
	note := func(f string, args ...any) {
		s := fmt.Sprintf(f, args...)
		report = append(report, s)
		log.Print("Автонастройка: " + s)
	}
	st := a.SetupStatus()
	for _, ch := range st.Checks {
		if ch.OK || ch.Fix == "" {
			continue
		}
		switch ch.Fix {
		case "cable":
			note("VB-Cable не найден — нажмите «Скачать и установить» во вкладке «Настройка»")
			continue
		case "launch":
			if winapi.SteamRunning() && !allowSteamRestart {
				note("параметр запуска: нужно перезапустить Steam — подтвердите во вкладке «Настройка»")
				continue
			}
		}
		if err := a.Fix(ch.Fix); err != nil {
			note("%s: %v", ch.Title, err)
		} else {
			note("%s ✓", ch.Title)
		}
	}
	a.UpdateConfig(func(c *config.Config) { c.SetupDone = true })
	return report
}

// Watchdog — фоновые автоматические проверки:
//   - аудио не запущено (нет кабеля / устройство отключили) → пробует перезапустить;
//   - поменяли бинд голосового чата в Dota 2 → обновляет ptt.key.
func (a *App) Watchdog() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	n := 0
	for range t.C {
		n++
		if !a.out.Running() {
			_, p, err := audio.Devices()
			if err == nil && audio.FindCable(p) != "" {
				a.Fix("audio")
			}
		}
		if n%3 == 0 {
			c := a.Config()
			if !c.PTT.AutoDetect {
				continue
			}
			root := winapi.SteamRoot()
			dota, _ := winapi.FindDota(root)
			key, _, err := winapi.FindVoiceKey(root, dota)
			if err == nil && key != c.PTT.Key {
				log.Printf("Кнопка голосового чата в Dota 2 изменилась: %s → %s", c.PTT.Key, key)
				a.UpdateConfig(func(c *config.Config) { c.PTT.Key = key })
			}
		}
	}
}
