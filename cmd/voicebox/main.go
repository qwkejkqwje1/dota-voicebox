// VoiceBox — саундпад и эффекты голоса через виртуальный кабель: звуки и голос в любой
// голосовой чат. Быстрое меню поверх игры, значок в трее, модуль Dota 2 (тревога «ганк на мид»,
// голосовые напоминания о рунах).
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/logbus"
	"github.com/qwkejkqwje1/dota-voicebox/internal/overlay"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/ui"
	"github.com/qwkejkqwje1/dota-voicebox/internal/update"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

var version = "dev"

// Окно WebView2 должно жить в главном потоке.
func init() { runtime.LockOSThread() }

func main() {
	exe, _ := os.Executable()
	defCfg := filepath.Join(filepath.Dir(exe), "config.json")

	cfgPath := flag.String("config", defCfg, "путь к config.json")
	listDev := flag.Bool("list-devices", false, "показать аудиоустройства и выйти")
	installGSI := flag.Bool("install-gsi", false, "установить GSI-конфиг в папку Dota 2 и выйти")
	exportDir := flag.String("export-sounds", "", "сохранить встроенные звуки в WAV в указанную папку и выйти")
	background := flag.Bool("background", false, "запуск без окна (автозагрузка)")
	showVer := flag.Bool("version", false, "версия")
	guardExe := flag.String("update-guard", "", "служебный: сторож установки обновления")
	postUpdate := flag.Bool("post-update", false, "служебный: первый запуск после обновления")
	waitRestart := flag.Bool("wait-restart", false, "служебный: дождаться выхода прошлой копии")
	flag.Parse()
	log.SetFlags(log.Ltime)

	switch {
	case *showVer:
		fmt.Println("VoiceBox", version)
		return
	case *listDev:
		if err := audio.ListDevices(); err != nil {
			log.Fatal(err)
		}
		return
	case *exportDir != "":
		os.MkdirAll(*exportDir, 0o755)
		for _, n := range sounds.BuiltinNames() {
			c, _ := sounds.Builtin(n)
			p := filepath.Join(*exportDir, n+".wav")
			if err := os.WriteFile(p, sounds.EncodeWAV16(c), 0o644); err != nil {
				log.Fatal(err)
			}
			fmt.Println(p)
		}
		return
	}

	abs, _ := filepath.Abs(*cfgPath)
	base := filepath.Dir(abs)
	sysLang := i18n.Detect(winapi.UILanguage())
	i18n.SetLang(sysLang)
	if _, err := config.EnsureFile(abs, sysLang); err != nil {
		fatal("Не удалось создать config.json: %v", err)
	}
	os.MkdirAll(filepath.Join(base, "sounds"), 0o755)
	cfg, err := config.Load(abs)
	if err != nil {
		fatal("Ошибка в config.json: %v", err)
	}
	i18n.SetLang(app.UILang(cfg))
	port := cfg.UI.Port
	if port == 0 {
		port = 3001
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	restartArgs := []string{"-config", abs}
	if *background {
		restartArgs = append(restartArgs, "-background")
	}
	if *guardExe != "" {
		update.Guard(*guardExe, restartArgs, func() bool { return pingExisting(addr) })
		return
	}
	if *waitRestart || *postUpdate {
		for i := 0; i < 100 && pingExisting(addr); i++ {
			time.Sleep(200 * time.Millisecond)
		}
	}

	// уже запущена? — показать окно существующей копии и выйти
	if pingExisting(addr) {
		winapi.AllowForeground() // разрешить той копии вывести окно на передний план
		http.Post("http://"+addr+"/api/show", "text/plain", nil)
		return
	}

	if *installGSI {
		dota, err := winapi.FindDota(winapi.SteamRoot())
		if err != nil {
			log.Fatal(err)
		}
		p, err := winapi.InstallGSI(dota, "http://"+cfg.GSI.Addr+"/", cfg.GSI.Token)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("GSI установлен:", p)
		return
	}

	// журнал: файл + интерфейс + консоль
	logs := logbus.New(500)
	logPath := filepath.Join(base, "voicebox.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 1<<20 {
		os.Remove(logPath)
	}
	lf, _ := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	writers := []io.Writer{logs, os.Stdout}
	if lf != nil {
		writers = append(writers, lf)
		defer lf.Close()
	}
	log.SetOutput(io.MultiWriter(writers...))
	log.Printf("VoiceBox %s запущен", version)
	winapi.SetHighPriority()
	go func() {
		if fixed := winapi.HealShortcuts(); len(fixed) > 0 {
			log.Printf("Программа переехала в другую папку — обновлены: %s", strings.Join(fixed, ", "))
		}
	}()

	eng := audio.NewEngine()
	a := app.New(abs, eng)
	a.Version = version

	showCh := make(chan struct{}, 1)
	quitCh := make(chan struct{}, 1)
	show := func() {
		if ui.FocusWindow() { // окно уже открыто — поднять его
			return
		}
		select {
		case showCh <- struct{}{}:
		default:
		}
	}
	quit := func() {
		select {
		case quitCh <- struct{}{}:
		default:
		}
		ui.CloseWindow()
	}
	a.OnShowUI = show

	// значок в трее, быстрое меню и уведомления поверх игры
	emoji := &overlay.FileEmoji{Dir: filepath.Join(base, "cache", "emoji")}
	rend := &overlay.Renderer{F: overlay.LoadFonts(), Emoji: emoji}
	var shell *winapi.Shell
	shell, err = winapi.StartShell(winapi.ShellOptions{
		Tooltip:  "VoiceBox",
		Menu:     &overlay.Controller{R: rend, Src: menuSource{a}},
		Wheel:    &overlay.WheelCtl{Src: wheelSource{a}},
		R:        rend,
		TrayMenu: func() []winapi.TrayItem { return trayItems(a) },
		OnTray:   func(id string) { trayAction(a, shell, id, show, quit) },
		OnMenuMoved: func(x, y float64) {
			if err := a.MoveQuickMenu(x, y); err != nil {
				log.Printf("быстрое меню: не удалось запомнить место: %v", err)
			}
		},
	})
	if err != nil {
		log.Printf("Значок в трее и быстрое меню недоступны: %v", err)
	}
	defer shell.Close()
	a.OnMenu = shell.ToggleMenu
	if shell != nil {
		a.OnWheel = shell.OpenWheel
		a.OnSay = shell.OpenSay
		a.OnReader = shell.OpenReader
	}
	a.OnToast = func(t app.Toast) { shell.Toast(overlay.ToastView{Emoji: t.Emoji, Text: t.Text, Sub: t.Sub}) }
	remote := &ui.Remote{App: a} // пульт с телефона (свой сервер в домашней сети, если включён)
	defer remote.Close()
	a.OnChange = func() {
		shell.RefreshMenu()
		shell.SetTooltip(trayTip(a))
		remote.Apply(a.Config())
	}

	a.AttachHotkeys()
	if err := a.Reload(); err != nil {
		fatal("Ошибка конфига: %v", err)
	}
	shell.SetTooltip(trayTip(a))
	go a.PTTLoop()
	go a.ClockLoop()
	keyCh := make(chan combo.Event, 512)
	if err := winapi.StartKeyHook(keyCh); err != nil {
		log.Printf("Перехват клавиш недоступен (скрипты, кнопки мыши, клавиши быстрого меню): %v", err)
	} else {
		go func() {
			for ev := range keyCh {
				a.KeyEvent(ev)
			}
		}()
	}
	go a.WatchConfig()
	go a.Watchdog()
	go a.ForegroundLoop()
	if !cfg.SetupDone {
		go a.AutoSetup(false)
	}
	defer a.StopGSI()

	upd := update.New(version, exe)
	a.UpdateStatus = func() any { return upd.Status() }
	token := ui.NewToken()
	if t := os.Getenv("VOICEBOX_DEV_TOKEN"); t != "" { // для разработки интерфейса в обычном браузере
		token = t
	}
	srv := &ui.Server{App: a, Engine: eng, Logs: logs, Token: token, Addr: addr, OnShow: show, OnQuit: quit,
		Updater: upd, RestartArgs: restartArgs, Emoji: emoji, Remote: remote, Exe: exe}
	if shell != nil {
		srv.OnMenuPreview = shell.OpenMenu
		srv.OnWheelPreview = func() { shell.OpenWheel(0) }
	}
	l, err := srv.Listen()
	if err != nil {
		fatal("Порт интерфейса %s занят: %v", addr, err)
	}
	go srv.Serve(l)
	log.Printf("Интерфейс: http://%s/", addr)
	if *postUpdate {
		log.Printf("Обновлено до %s", version)
		update.MarkHealthy(exe)
	}
	go updateLoop(a, upd, restartArgs, quit)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() { <-sig; quit() }()

	if !*background && cfg.UI.OpenOnStart {
		show()
	}
	defer func() {
		eng.Close()
		winapi.CloseTTS() // фоновый PowerShell синтеза речи
		log.Print("Выход")
	}()
	for {
		select {
		case <-quitCh:
			return
		case <-showCh:
			if !ui.OpenWindow(srv.URL()) {
				winapi.OpenURL(srv.URL())
				continue // в браузере: работаем, пока не нажмут «Выход»
			}
			// окно закрыто
			select {
			case <-showCh: // сбросить запросы, пришедшие пока окно было открыто
			default:
			}
			if len(quitCh) > 0 {
				return
			}
			c := a.Config()
			if !c.UI.KeepRunningOnClose {
				return
			}
			log.Print("Окно закрыто — VoiceBox работает в фоне (значок в трее)")
			if !c.UI.TrayHintShown {
				hint := i18n.T("Значок — в области уведомлений рядом с часами.")
				if k := keys.Display(a.HotkeyMap()["menu"]); k != "" {
					hint += " " + fmt.Sprintf(i18n.T("Быстрое меню в игре: %s"), k)
				}
				shell.Balloon(i18n.T("VoiceBox работает в фоне"), hint)
				a.UpdateConfig(func(c *config.Config) { c.UI.TrayHintShown = true })
			}
		}
	}
}

// updateLoop — проверка обновлений при запуске и раз в 6 часов; автоустановка вне матча.
func updateLoop(a *app.App, u *update.Updater, args []string, quit func()) {
	time.Sleep(20 * time.Second)
	for {
		c := a.Config()
		if c.Update.AutoCheck {
			if _, err := u.Check(); err != nil {
				log.Printf("Обновления: %v", err)
			} else if st := u.Status(); st.Available && st.Latest != c.Update.Skip {
				log.Printf("Доступна новая версия %s (у вас %s)", st.Latest, st.Current)
				if c.Update.AutoInstall {
					// не обновляемся посреди матча: ждём, пока игра не идёт
					for a.Game().Clock.OK {
						time.Sleep(time.Minute)
					}
					if err := u.Download(); err == nil {
						if err := u.Install(args); err == nil {
							quit()
							return
						} else {
							log.Printf("Обновление: %v", err)
						}
					}
				}
			}
		}
		time.Sleep(6 * time.Hour)
	}
}

func pingExisting(addr string) bool {
	cl := http.Client{Timeout: 700 * time.Millisecond}
	r, err := cl.Get("http://" + addr + "/api/ping")
	if err != nil {
		return false
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return string(b) == "voicebox"
}

func fatal(f string, args ...any) {
	msg := fmt.Sprintf(f, args...)
	log.Print(msg)
	winapi.Alert("VoiceBox", i18n.T(msg))
	os.Exit(1)
}
