//go:build windows

package ui

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	webview2 "github.com/jchv/go-webview2"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	pGetDpiForSystem     = user32.NewProc("GetDpiForSystem")
	pGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	pShowWindow          = user32.NewProc("ShowWindow")
	pIsIconic            = user32.NewProc("IsIconic")
	pSetForegroundWindow = user32.NewProc("SetForegroundWindow")

	winMu  sync.Mutex
	winCur webview2.WebView
)

// Title — заголовок окна.
var Title = "VoiceBox"

func systemDPI() int {
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d >= 96 {
			return int(d)
		}
	}
	return 96
}

// OpenWindow открывает окно интерфейса и блокируется до его закрытия.
// Должна вызываться из потока, закреплённого через runtime.LockOSThread.
// Возвращает false, если WebView2 недоступен (тогда открываем браузер).
func OpenWindow(url string) bool {
	data := filepath.Join(os.Getenv("LOCALAPPDATA"), "DotaVoiceBox", "webview")
	dpi := systemDPI()
	sc := func(v int) int { return v * dpi / 96 }
	sw, _, _ := pGetSystemMetrics.Call(16) // SM_CXFULLSCREEN — рабочая область
	sh, _, _ := pGetSystemMetrics.Call(17)
	w0, h0 := min(sc(1200), int(sw)), min(sc(820), int(sh))
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  data,
		WindowOptions: webview2.WindowOptions{
			Title: Title, Width: uint(w0), Height: uint(h0), IconId: 1, Center: true,
		},
	})
	if w == nil {
		log.Print("WebView2 недоступен — открываю интерфейс в браузере")
		return false
	}
	defer w.Destroy()
	w.SetSize(min(sc(940), w0), min(sc(620), h0), webview2.HintMin)
	w.Navigate(url)
	winMu.Lock()
	winCur = w
	winMu.Unlock()
	defer func() {
		winMu.Lock()
		winCur = nil
		winMu.Unlock()
	}()
	w.Run()
	return true
}

// FocusWindow поднимает уже открытое окно. false — окна нет.
func FocusWindow() bool {
	winMu.Lock()
	w := winCur
	winMu.Unlock()
	if w == nil {
		return false
	}
	h := uintptr(w.Window())
	if r, _, _ := pIsIconic.Call(h); r != 0 {
		pShowWindow.Call(h, 9) // SW_RESTORE
	} else {
		pShowWindow.Call(h, 5) // SW_SHOW
	}
	pSetForegroundWindow.Call(h)
	return true
}

// CloseWindow закрывает окно (например, «Выход» из трея).
func CloseWindow() {
	winMu.Lock()
	w := winCur
	winMu.Unlock()
	if w != nil {
		w.Destroy()
	}
}
