// Dota VoiceBox — саундпад + пресеты голоса + таймеры рун для Dota 2.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/qwkejkqwje1/dota-voicebox/internal/app"
	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

var version = "dev"

func main() {
	exe, _ := os.Executable()
	defCfg := filepath.Join(filepath.Dir(exe), "config.json")

	cfgPath := flag.String("config", defCfg, "путь к config.json")
	listDev := flag.Bool("list-devices", false, "показать аудиоустройства и выйти")
	installGSI := flag.Bool("install-gsi", false, "установить GSI-конфиг в папку Dota 2 и выйти")
	exportDir := flag.String("export-sounds", "", "сохранить встроенные звуки в WAV в указанную папку и выйти")
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
	if created, err := config.EnsureFile(abs); err != nil {
		log.Fatal(err)
	} else if created {
		log.Printf("Создан %s — откройте его и укажите ptt.key (кнопка голосового чата в Dota 2).", abs)
	}
	os.MkdirAll(filepath.Join(filepath.Dir(abs), "sounds"), 0o755)
	cfg, err := config.Load(abs)
	if err != nil {
		log.Fatal(err)
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
		fmt.Println("Добавьте в параметры запуска Dota 2: -gamestateintegration")
		return
	}

	eng := audio.NewEngine(audio.Options{
		MicDevice: cfg.Devices.Mic, VoiceOut: cfg.Devices.VoiceOut, MonitorOut: cfg.Devices.Monitor,
		MicGain: cfg.MicGain, Ducking: cfg.Ducking, FxOnSounds: cfg.FxOnSounds, MonitorVolume: cfg.MonitorVolume,
	})
	if err := eng.Start(); err != nil {
		log.Printf("Ошибка аудио: %v", err)
		log.Print("Проверьте устройства: voicebox.exe -list-devices, и поле devices в config.json")
		os.Exit(1)
	}
	defer eng.Close()

	a := app.New(abs, eng)
	a.AttachHotkeys()
	if err := a.Reload(); err != nil {
		log.Fatal(err)
	}
	go a.PTTLoop()
	go a.WatchConfig()

	if cfg.GSI.Enabled {
		srv := &gsi.Server{Addr: cfg.GSI.Addr, Token: cfg.GSI.Token, OnState: a.OnGSI}
		go func() {
			log.Printf("GSI: жду Dota 2 на http://%s/", cfg.GSI.Addr)
			if err := srv.Run(); err != nil {
				log.Printf("GSI сервер: %v", err)
			}
		}()
	}

	fmt.Println(a.Summary())
	log.Print("Готово. Ctrl+C — выход.")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
