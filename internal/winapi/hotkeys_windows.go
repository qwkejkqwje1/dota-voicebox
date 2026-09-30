//go:build windows

// Package winapi — глобальные горячие клавиши, эмуляция PTT, TTS и поиск Dota 2.
package winapi

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	pRegisterHotKey     = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey   = user32.NewProc("UnregisterHotKey")
	pGetMessageW        = user32.NewProc("GetMessageW")
	pPostThreadMessageW = user32.NewProc("PostThreadMessageW")
	pSendInput          = user32.NewProc("SendInput")
	pMapVirtualKeyW     = user32.NewProc("MapVirtualKeyW")
	pGetAsyncKeyState   = user32.NewProc("GetAsyncKeyState")
	pGetCurrentThreadId = kernel32.NewProc("GetCurrentThreadId")
)

const (
	wmHotkey = 0x0312
	wmApp    = 0x8000
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

// Hotkeys — менеджер глобальных горячих клавиш (работает поверх игры).
type Hotkeys struct {
	mu      sync.Mutex
	tid     uintptr
	pending []keys.Combo
	onPress func(combo string)
	ready   chan struct{}
	result  chan []error
	current map[int]keys.Combo
}

func NewHotkeys(onPress func(combo string)) *Hotkeys {
	h := &Hotkeys{onPress: onPress, ready: make(chan struct{}), result: make(chan []error, 1), current: map[int]keys.Combo{}}
	go h.loop()
	<-h.ready
	return h
}

// Set заменяет набор горячих клавиш. Возвращает ошибки (например, клавиша занята другой программой).
func (h *Hotkeys) Set(combos []keys.Combo) []error {
	h.mu.Lock()
	h.pending = combos
	h.mu.Unlock()
	pPostThreadMessageW.Call(h.tid, wmApp, 0, 0)
	return <-h.result
}

func (h *Hotkeys) loop() {
	runtime.LockOSThread()
	h.tid, _, _ = pGetCurrentThreadId.Call()
	close(h.ready)
	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		switch m.message {
		case wmHotkey:
			if c, ok := h.current[int(m.wParam)]; ok {
				go h.onPress(c.Text)
			}
		case wmApp:
			h.mu.Lock()
			combos := h.pending
			h.mu.Unlock()
			for id := range h.current {
				pUnregisterHotKey.Call(0, uintptr(id))
			}
			h.current = map[int]keys.Combo{}
			var errs []error
			for i, c := range combos {
				id := i + 1
				r, _, e := pRegisterHotKey.Call(0, uintptr(id), uintptr(c.Mods|keys.ModNoRepeat), uintptr(c.VK))
				if r == 0 {
					errs = append(errs, fmt.Errorf("клавиша %s не зарегистрирована (занята другой программой?): %v", c.Text, e))
					continue
				}
				h.current[id] = c
			}
			h.result <- errs
		}
	}
}

// ---------- эмуляция клавиши голосового чата ----------

type keyboardInput struct {
	typ       uint32
	_         uint32
	wVk       uint16
	wScan     uint16
	dwFlags   uint32
	time      uint32
	extraInfo uintptr
	_         [8]byte
}

const (
	inputKeyboard     = 1
	inputMouse        = 0
	keyeventfKeyUp    = 0x0002
	keyeventfScancode = 0x0008
	keyeventfExtended = 0x0001
)

type mouseInput struct {
	typ       uint32
	_         uint32
	dx, dy    int32
	mouseData uint32
	dwFlags   uint32
	time      uint32
	extraInfo uintptr
}

// PressKey нажимает (down=true) или отпускает клавишу. Используются скан-коды —
// игры на DirectInput/RawInput их видят. Mouse4/Mouse5 эмулируются как кнопки мыши.
func PressKey(vk uint32, down bool) error {
	if vk == 0x05 || vk == 0x06 { // XBUTTON1/2
		const down_, up_ = 0x0080, 0x0100
		in := mouseInput{typ: inputMouse, mouseData: 1}
		if vk == 0x06 {
			in.mouseData = 2
		}
		in.dwFlags = up_
		if down {
			in.dwFlags = down_
		}
		r, _, e := pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
		if r == 0 {
			return e
		}
		return nil
	}
	scan, _, _ := pMapVirtualKeyW.Call(uintptr(vk), 0)
	in := keyboardInput{typ: inputKeyboard, wScan: uint16(scan), dwFlags: keyeventfScancode}
	switch vk { // расширенные клавиши
	case 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x2D, 0x2E, 0xA3, 0xA5, 0x6F:
		in.dwFlags |= keyeventfExtended
	}
	if !down {
		in.dwFlags |= keyeventfKeyUp
	}
	r, _, e := pSendInput.Call(1, uintptr(unsafe.Pointer(&in)), unsafe.Sizeof(in))
	if r == 0 {
		return e
	}
	return nil
}

// IsKeyDown — зажата ли клавиша прямо сейчас.
func IsKeyDown(vk uint32) bool {
	r, _, _ := pGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}
