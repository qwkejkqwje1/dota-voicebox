// Dota VoiceBox — саундпад + пресеты голоса + таймеры рун для Dota 2.
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
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/logbus"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/ui"
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
	flag.Parse()
	log.SetFlags(log.Ltime)

	switch {
	case *showVer:
		fmt.Println("Dota VoiceBox", version)
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
	if _, err := config.EnsureFile(abs); err != nil {
		fatal("Не удалось создать config.json: %v", err)
	}
	os.MkdirAll(filepath.Join(base, "sounds"), 0o755)
	cfg, err := config.Load(abs)
	if err != nil {
		fatal("Ошибка в config.json: %v", err)
	}
	port := cfg.UI.Port
	if port == 0 {
		port = 3001
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	// уже запущена? — показать окно существующей копии и выйти
	if pingExisting(addr) {
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
	log.Printf("Dota VoiceBox %s запущен", version)
	winapi.SetHighPriority()

	eng := audio.NewEngine()
	a := app.New(abs, eng)
	a.Version = version
	a.AttachHotkeys()
	if err := a.Reload(); err != nil {
		fatal("Ошибка конфига: %v", err)
	}
	go a.PTTLoop()
	go a.ClockLoop()
	keyCh := make(chan combo.Event, 512)
	if err := winapi.StartKeyHook(keyCh); err != nil {
		log.Printf("Перехват клавиш для скриптов недоступен: %v", err)
	} else {
		go func() {
			for ev := range keyCh {
				a.KeyEvent(ev)
			}
		}()
	}
	go a.WatchConfig()
	go a.Watchdog()
	if !cfg.SetupDone {
		go a.AutoSetup(false)
	}
	if cfg.GSI.Enabled {
		srv := &gsi.Server{Addr: cfg.GSI.Addr, Token: cfg.GSI.Token, OnState: a.OnGSI}
		go func() {
			log.Printf("GSI: жду Dota 2 на http://%s/", cfg.GSI.Addr)
			if err := srv.Run(); err != nil {
				log.Printf("GSI сервер: %v", err)
			}
		}()
	}

	showCh := make(chan struct{}, 1)
	quitCh := make(chan struct{}, 1)
	show := func() {
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
	}
	a.OnShowUI = show

	srv := &ui.Server{App: a, Engine: eng, Logs: logs, Token: ui.NewToken(), Addr: addr, OnShow: show, OnQuit: quit}
	l, err := srv.Listen()
	if err != nil {
		fatal("Порт интерфейса %s занят: %v", addr, err)
	}
	go srv.Serve(l)
	log.Printf("Интерфейс: http://%s/", addr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() { <-sig; quit() }()

	if !*background && cfg.UI.OpenOnStart {
		show()
	}
	defer func() {
		eng.Close()
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
			if !a.Config().UI.KeepRunningOnClose {
				return
			}
			log.Print("Окно закрыто — VoiceBox работает в фоне (Ctrl+Alt+V или повторный запуск — открыть)")
		}
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
	winapi.Alert("Dota VoiceBox", msg)
	os.Exit(1)
}
