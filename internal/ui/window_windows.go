//go:build windows

package ui

import (
	"log"
	"os"
	"path/filepath"

	webview2 "github.com/jchv/go-webview2"
)

// OpenWindow открывает окно интерфейса и блокируется до его закрытия.
// Должна вызываться из потока, закреплённого через runtime.LockOSThread.
// Возвращает false, если WebView2 недоступен (тогда открываем браузер).
func OpenWindow(url string) bool {
	data := filepath.Join(os.Getenv("LOCALAPPDATA"), "DotaVoiceBox", "webview")
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  data,
		WindowOptions: webview2.WindowOptions{
			Title: "Dota VoiceBox", Width: 1180, Height: 800, IconId: 1, Center: true,
		},
	})
	if w == nil {
		log.Print("WebView2 недоступен — открываю интерфейс в браузере")
		return false
	}
	defer w.Destroy()
	w.SetSize(980, 640, webview2.HintMin)
	w.Navigate(url)
	w.Run()
	return true
}
