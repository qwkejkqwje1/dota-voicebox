//go:build windows

package winapi

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
)

var (
	pSetWindowsHookExW = user32.NewProc("SetWindowsHookExW")
	pCallNextHookEx    = user32.NewProc("CallNextHookEx")
	pGetModuleHandleW  = kernel32.NewProc("GetModuleHandleW")
)

const (
	whKeyboardLL  = 13
	whMouseLL     = 14
	llkhfInjected = 0x10
	llmhfInjected = 0x01
)

type kbdLL struct {
	vk, scan, flags, time uint32
	extra                 uintptr
}

type msLL struct {
	x, y                   int32
	mouseData, flags, time uint32
	extra                  uintptr
}

type hookFilters struct {
	key   func(vk, scan uint32, down bool) bool
	mouse func(msg uintptr, x, y int32, data uint32) bool
}

var hookFilter atomic.Pointer[hookFilters]

// SetHookFilter — обработчики, которые могут «съесть» нажатие (быстрое меню поверх игры).
// Вызываются в потоке перехватчика — должны отвечать мгновенно.
func SetHookFilter(key func(vk, scan uint32, down bool) bool, mouse func(msg uintptr, x, y int32, data uint32) bool) {
	if key == nil && mouse == nil {
		hookFilter.Store(nil)
		return
	}
	hookFilter.Store(&hookFilters{key: key, mouse: mouse})
}

// StartKeyHook запускает глобальный перехват клавиатуры и боковых кнопок мыши.
// Нажатия не блокируются (игра их тоже получает), кроме клавиш открытого быстрого меню. Эмулированные программой
// нажатия (наш PTT) игнорируются. Обработчик вызывается в отдельной горутине.
func StartKeyHook(out chan<- combo.Event) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		send := func(vk uint32, down bool) {
			select {
			case out <- combo.Event{VK: vk, Down: down, T: time.Now()}:
			default: // переполнение — пропускаем, чтобы не тормозить систему
			}
		}
		kb := syscall.NewCallback(func(code int, wparam uintptr, k *kbdLL) uintptr {
			if code >= 0 && k != nil {
				if k.flags&llkhfInjected == 0 {
					down := wparam == 0x0100 || wparam == 0x0104
					up := wparam == 0x0101 || wparam == 0x0105
					if f := hookFilter.Load(); f != nil && f.key != nil && (down || up) && f.key(k.vk, k.scan, down) {
						return 1
					}
					switch wparam {
					case 0x0100, 0x0104: // WM_KEYDOWN, WM_SYSKEYDOWN
						send(k.vk, true)
					case 0x0101, 0x0105:
						send(k.vk, false)
					}
				}
			}
			r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wparam, uintptr(unsafe.Pointer(k)))
			return r
		})
		ms := syscall.NewCallback(func(code int, wparam uintptr, m *msLL) uintptr {
			if code >= 0 && m != nil {
				if m.flags&llmhfInjected == 0 {
					if f := hookFilter.Load(); f != nil && f.mouse != nil && wparam != 0x0200 && f.mouse(wparam, m.x, m.y, m.mouseData) {
						return 1
					}
					btn := uint32(0)
					switch m.mouseData >> 16 {
					case 1:
						btn = 0x05
					case 2:
						btn = 0x06
					}
					switch wparam {
					case 0x020B: // WM_XBUTTONDOWN
						if btn != 0 {
							send(btn, true)
						}
					case 0x020C:
						if btn != 0 {
							send(btn, false)
						}
					case 0x0207: // WM_MBUTTONDOWN
						send(0x04, true)
					case 0x0208:
						send(0x04, false)
					}
				}
			}
			r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wparam, uintptr(unsafe.Pointer(m)))
			return r
		})
		mod, _, _ := pGetModuleHandleW.Call(0)
		h1, _, e1 := pSetWindowsHookExW.Call(whKeyboardLL, kb, mod, 0)
		if h1 == 0 {
			errc <- e1
			return
		}
		pSetWindowsHookExW.Call(whMouseLL, ms, mod, 0)
		errc <- nil
		var m msg
		for {
			r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
		}
	}()
	return <-errc
}
