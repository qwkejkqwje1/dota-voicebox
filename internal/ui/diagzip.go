package ui

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// diagBundle собирает диагностический пакет одним файлом:
// журнал, конфиг, устройства, проверки, статус, результаты тестов (присылает интерфейс).
func (s *Server) diagBundle(r *http.Request) (any, error) {
	var req struct {
		Report json.RawMessage `json:"report"`
	}
	decode(r, &req)
	dir := filepath.Join(s.App.BaseDir, "diagnostics")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := filepath.Join(dir, "voicebox-diag-"+time.Now().Format("20060102-150405")+".zip")
	f, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(f)
	put := func(n string, v any) {
		w, err := zw.Create(n)
		if err != nil {
			return
		}
		switch x := v.(type) {
		case string:
			io.WriteString(w, x)
		case []byte:
			w.Write(x)
		default:
			e := json.NewEncoder(w)
			e.SetIndent("", "  ")
			e.Encode(x)
		}
	}
	put("version.txt", fmt.Sprintf("VoiceBox %s\n%s/%s %s\n%s\n", s.App.Version, runtime.GOOS, runtime.GOARCH, runtime.Version(), time.Now().Format(time.RFC3339)))
	put("log.txt", strings.Join(s.Logs.Lines(), "\n"))
	put("config.json", s.App.Config())
	capture, playback, derr := audio.Devices()
	eps, eerr := winapi.AudioEndpoints()
	put("devices.json", map[string]any{"capture": capture, "playback": playback, "error": fmt.Sprint(derr), "windows": eps, "windows_error": fmt.Sprint(eerr)})
	put("setup.json", s.App.SetupStatus())
	put("status.json", s.App.Status())
	if len(req.Report) > 0 {
		put("tests.json", []byte(req.Report))
	}
	if exe, err := os.Executable(); err == nil {
		if b, err := os.ReadFile(exe + ".update.log"); err == nil {
			put("update.log", b)
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	winapi.OpenURL(dir)
	return map[string]string{"file": name}, nil
}
