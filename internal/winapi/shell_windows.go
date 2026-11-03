//go:build windows

package winapi

import (
	"image"
	"log"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/qwkejkqwje1/dota-voicebox/internal/overlay"
)

// Оболочка Windows: значок в трее, быстрое меню поверх игры (layered window без фокуса —
// игра остаётся активной, клавиши меню ловит низкоуровневый перехватчик) и уведомления.
// Все окна живут в одном своём потоке с очередью сообщений.

var (
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	pRegisterClassExW         = user32.NewProc("RegisterClassExW")
	pCreateWindowExW          = user32.NewProc("CreateWindowExW")
	pDefWindowProcW           = user32.NewProc("DefWindowProcW")
	pDestroyWindow            = user32.NewProc("DestroyWindow")
	pShowWindow               = user32.NewProc("ShowWindow")
	pSetWindowPos             = user32.NewProc("SetWindowPos")
	pUpdateLayeredWindow      = user32.NewProc("UpdateLayeredWindow")
	pGetDC                    = user32.NewProc("GetDC")
	pReleaseDC                = user32.NewProc("ReleaseDC")
	pPostMessageW             = user32.NewProc("PostMessageW")
	pTranslateMessage         = user32.NewProc("TranslateMessage")
	pDispatchMessageW         = user32.NewProc("DispatchMessageW")
	pSetTimer                 = user32.NewProc("SetTimer")
	pKillTimer                = user32.NewProc("KillTimer")
	pGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	pMonitorFromWindow        = user32.NewProc("MonitorFromWindow")
	pMonitorFromPoint         = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfoW          = user32.NewProc("GetMonitorInfoW")
	pGetCursorPos             = user32.NewProc("GetCursorPos")
	pTrackMouseEvent          = user32.NewProc("TrackMouseEvent")
	pLoadCursorW              = user32.NewProc("LoadCursorW")
	pSetCursor                = user32.NewProc("SetCursor")
	pSetCapture               = user32.NewProc("SetCapture")
	pReleaseCapture           = user32.NewProc("ReleaseCapture")
	pCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	pAppendMenuW              = user32.NewProc("AppendMenuW")
	pTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	pDestroyMenu              = user32.NewProc("DestroyMenu")
	pSetMenuDefaultItem       = user32.NewProc("SetMenuDefaultItem")
	pSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	pLoadImageW               = user32.NewProc("LoadImageW")
	pRegisterWindowMessageW   = user32.NewProc("RegisterWindowMessageW")
	pPostQuitMessage          = user32.NewProc("PostQuitMessage")
	pGetSystemMetrics         = user32.NewProc("GetSystemMetrics")
	pGetWindowRect            = user32.NewProc("GetWindowRect")
	pGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	pCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	pCreateDIBSection         = gdi32.NewProc("CreateDIBSection")
	pSelectObject             = gdi32.NewProc("SelectObject")
	pDeleteObject             = gdi32.NewProc("DeleteObject")
	pDeleteDC                 = gdi32.NewProc("DeleteDC")
	pGetDeviceCaps            = gdi32.NewProc("GetDeviceCaps")
	pShellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
	pGetDpiForMonitor         = shcore.NewProc("GetDpiForMonitor")
	pLoadIconMetric           = comctl32.NewProc("LoadIconMetric")
	pGetCurrentProcessId      = kernel32.NewProc("GetCurrentProcessId")
)

const (
	wsPopup         = 0x80000000
	wsExLayered     = 0x00080000
	wsExTopmost     = 0x00000008
	wsExToolwindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	wsExTransparent = 0x00000020

	swHide = 0

	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoZOrder      = 0x0004
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	hwndTopmost      = ^uintptr(0) // (HWND)-1
	ulwAlpha         = 2
	acSrcAlpha       = 1
	monitorNearest   = 2
	maNoActivate     = 3
	tmeLeave         = 2
	idcArrow         = 32512
	idcHand          = 32649
	idcSizeAll       = 32646
	logPixelsX       = 88
	smCxSmIcon       = 49
	smCySmIcon       = 50
	imageIcon        = 1
	lrShared         = 0x8000
	wmNull           = 0x0000
	wmClose          = 0x0010
	wmSetCursor      = 0x0020
	wmMouseActivate  = 0x0021
	wmContextMenu    = 0x007B
	wmTimer          = 0x0113
	wmMouseMove      = 0x0200
	wmLButtonDown    = 0x0201
	wmLButtonUp      = 0x0202
	wmLButtonDblClk  = 0x0203
	wmRButtonDown    = 0x0204
	wmRButtonUp      = 0x0205
	wmMButtonDown    = 0x0207
	wmMButtonUp      = 0x0208
	wmMouseWheel     = 0x020A
	wmXButtonDown    = 0x020B
	wmXButtonUp      = 0x020C
	wmMouseLeave     = 0x02A3
	wmCaptureChanged = 0x0215

	nimAdd     = 0
	nimModify  = 1
	nimDelete  = 2
	nifMessage = 0x01
	nifIcon    = 0x02
	nifTip     = 0x04
	nifInfo    = 0x10
	niifInfo   = 0x01

	mfString       = 0x0000
	mfGrayed       = 0x0001
	mfChecked      = 0x0008
	mfPopup        = 0x0010
	mfSeparator    = 0x0800
	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	// свои сообщения скрытого окна
	wmShellOpen    = wmApp + 0x21
	wmShellToggle  = wmApp + 0x22
	wmShellClose   = wmApp + 0x23
	wmShellSync    = wmApp + 0x24 // состояние меню изменилось — перерисовать или спрятать
	wmShellRefresh = wmApp + 0x25 // приложение что-то сделало — перечитать меню
	wmShellToast   = wmApp + 0x26
	wmShellTip     = wmApp + 0x27
	wmShellBalloon = wmApp + 0x28
	wmShellQuit    = wmApp + 0x29
	wmShellWheel   = wmApp + 0x2A // открыть колесо (wParam — клавиша, которую держат)
	wmShellWSync   = wmApp + 0x2B // колесо: перерисовать или спрятать
	wmShellSay     = wmApp + 0x2C // меню сразу в режиме набора текста
	wmTray         = wmApp + 0x30

	timerRefresh = 1
	timerToast   = 2
	timerFade    = 3
	timerTray    = 4
	timerDrag    = 5 // пока тащат меню: курсор и кнопку опрашиваем сами (окно меню не активно, захват мыши ненадёжен)
	timerWheel   = 6 // пока открыто колесо: курсор, удержание клавиши, тайм-аут

	smSwapButton = 23
)

type point32 struct{ X, Y int32 }
type size32 struct{ W, H int32 }
type rect32 struct{ L, T, R, B int32 }

func (r rect32) rect() image.Rectangle { return image.Rect(int(r.L), int(r.T), int(r.R), int(r.B)) }

type monitorInfo struct {
	Size    uint32
	Monitor rect32
	Work    rect32
	Flags   uint32
}

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type trackMouseEvent struct {
	size      uint32
	flags     uint32
	hwnd      uintptr
	hoverTime uint32
}

type bitmapInfoHeader struct {
	Size                         uint32
	Width, Height                int32
	Planes, BitCount             uint16
	Compression, SizeImage       uint32
	XPelsPerMeter, YPelsPerMeter int32
	ClrUsed, ClrImportant        uint32
}

type bitmapInfo struct {
	H      bitmapInfoHeader
	Colors [1]uint32
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     uintptr
}

// Shell — значок в трее, быстрое меню и уведомления.
type Shell struct {
	o      ShellOptions
	hwnd   uintptr // скрытое окно: значок, сообщения, таймеры
	menuW  uintptr
	toastW uintptr
	wheelW uintptr
	ready  chan error
	done   chan struct{}
	acts   chan func()

	mu       sync.Mutex // поля ниже трогают другие потоки
	menuRect image.Rectangle
	toastQ   *overlay.ToastView
	tip      string
	balloon  [2]string
	pressed  map[uint32]bool // клавиши, съеденные меню: ждём отпускания
	keyDown  map[uint32]bool // что сейчас нажато (по перехватчику): клавиши и кнопки мыши
	mouseEat map[uint32]bool // нажатия кнопок мыши, съеденные колесом: съесть и отпускание
	closedAt atomic.Int64    // когда меню закрыла «чужая» клавиша (чтобы она же не открыла его снова)

	// только поток окон
	shown      bool
	origin     image.Point
	size       image.Point // размер картинки меню при последней отрисовке
	ax, ay     float64     // какой край меню держать при смене размера (overlay.Align)
	layout     overlay.Layout
	area       image.Rectangle
	scale      float64
	cursor     image.Point
	tracking   bool
	hand       bool
	moveCur    bool // курсор над ручкой ⠿
	drag       menuDrag
	trayAdded  bool
	icon       uintptr
	taskbarMsg uintptr
	toastAlpha int
	trayIDs    []string
	wheel      wheelWin
}

// wheelWin — окно колеса (только поток окон).
type wheelWin struct {
	shown bool
	pos   image.Point
	base  *image.RGBA // колесо без стрелки: перерисовываем, только когда сменился выбор
	buf   *image.RGBA
	rev   int
}

// menuDrag — меню тащат мышью (за ручку ⠿ или фон).
type menuDrag struct {
	on, moved   bool
	start, orig image.Point // курсор и угол меню в начале
}

var (
	shellInst atomic.Pointer[Shell]
	shellProc = syscall.NewCallback(shellWndProc)
)

func wstr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	return p
}

func copyUTF16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(strings.ReplaceAll(s, "\x00", ""))
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	copy(dst, u)
}

// StartShell создаёт окна в отдельном потоке и подключается к перехватчику клавиш.
func StartShell(o ShellOptions) (*Shell, error) {
	s := &Shell{o: o, ready: make(chan error, 1), done: make(chan struct{}), acts: make(chan func(), 32),
		pressed: map[uint32]bool{}, keyDown: map[uint32]bool{}, mouseEat: map[uint32]bool{}, scale: 1, tip: o.Tooltip}
	shellInst.Store(s)
	go s.loop()
	if err := <-s.ready; err != nil {
		shellInst.Store(nil)
		return nil, err
	}
	go s.worker()
	if o.Menu != nil || o.Wheel != nil {
		SetHookFilter(s.filterKey, s.filterMouse)
	}
	return s, nil
}

func (s *Shell) create(ex uint32, cls *uint16, title string) uintptr {
	inst, _, _ := pGetModuleHandleW.Call(0)
	h, _, _ := pCreateWindowExW.Call(uintptr(ex), uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(wstr(title))),
		wsPopup, 0, 0, 0, 0, 0, 0, inst, 0)
	return h
}

func (s *Shell) loop() {
	runtime.LockOSThread()
	defer close(s.done)
	inst, _, _ := pGetModuleHandleW.Call(0)
	cls := wstr("VoiceBoxShell")
	wc := wndClassEx{wndProc: shellProc, instance: inst, className: cls}
	wc.size = uint32(unsafe.Sizeof(wc))
	wc.cursor, _, _ = pLoadCursorW.Call(0, idcArrow)
	if r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		s.ready <- e
		return
	}
	s.hwnd = s.create(wsExToolwindow, cls, "VoiceBox")
	s.menuW = s.create(wsExLayered|wsExTopmost|wsExToolwindow|wsExNoActivate, cls, "VoiceBox — быстрое меню")
	s.toastW = s.create(wsExLayered|wsExTopmost|wsExToolwindow|wsExNoActivate|wsExTransparent, cls, "VoiceBox — уведомление")
	// колесо пропускает мышь насквозь: курсор опрашиваем сами, щелчки ловит перехватчик
	s.wheelW = s.create(wsExLayered|wsExTopmost|wsExToolwindow|wsExNoActivate|wsExTransparent, cls, "VoiceBox — колесо звуков")
	if s.hwnd == 0 || s.menuW == 0 || s.toastW == 0 || s.wheelW == 0 {
		s.ready <- syscall.GetLastError()
		return
	}
	s.taskbarMsg, _, _ = pRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(wstr("TaskbarCreated"))))
	s.icon = loadSmallIcon(inst)
	s.addTray()
	if !s.trayAdded { // Проводник ещё не запустился (автозагрузка) — пробуем позже
		pSetTimer.Call(s.hwnd, timerTray, 3000, 0)
	}
	s.ready <- nil
	var m msg
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func loadSmallIcon(inst uintptr) uintptr {
	var h uintptr
	if pLoadIconMetric.Find() == nil {
		if r, _, _ := pLoadIconMetric.Call(inst, 1, 0, uintptr(unsafe.Pointer(&h))); r == 0 && h != 0 {
			return h
		}
	}
	cx, _, _ := pGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := pGetSystemMetrics.Call(smCySmIcon)
	h, _, _ = pLoadImageW.Call(inst, 1, imageIcon, cx, cy, lrShared)
	if h == 0 {
		h, _, _ = pLoadImageW.Call(0, 32512, imageIcon, cx, cy, lrShared) // IDI_APPLICATION
	}
	return h
}

func shellWndProc(hwnd, m, wp, lp uintptr) uintptr {
	if s := shellInst.Load(); s != nil && hwnd != 0 {
		if r, ok := s.handle(hwnd, uint32(m), wp, lp); ok {
			return r
		}
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

func (s *Shell) post(m uint32) {
	if s.hwnd != 0 {
		pPostMessageW.Call(s.hwnd, uintptr(m), 0, 0)
	}
}

func lparamXY(lp uintptr) (int, int) { return int(int16(lp & 0xFFFF)), int(int16((lp >> 16) & 0xFFFF)) }

func (s *Shell) handle(hwnd uintptr, m uint32, wp, lp uintptr) (uintptr, bool) {
	c := s.o.Menu
	if hwnd == s.menuW && s.menuW != 0 {
		switch m {
		case wmMouseActivate:
			return maNoActivate, true
		case wmSetCursor:
			setCursor(s.cursorID())
			return 1, true
		case wmMouseMove:
			if c == nil {
				return 0, true
			}
			if s.drag.on {
				if leftDown() {
					s.dragMove()
				} else {
					s.dragEnd()
				}
				return 0, true
			}
			if !s.tracking {
				t := trackMouseEvent{flags: tmeLeave, hwnd: s.menuW}
				t.size = uint32(unsafe.Sizeof(t))
				pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&t)))
				s.tracking = true
			}
			kind, i := s.layout.HitTest(lparamXY(lp))
			s.hand = kind == "row" || kind == "tab" || kind == "prev" || kind == "next"
			s.moveCur = kind == "grip"
			if kind != "row" {
				i = -1
			}
			if c.HoverAt(i) {
				s.drawMenu()
			}
			return 0, true
		case wmMouseLeave:
			s.tracking, s.hand, s.moveCur = false, false, false
			if c != nil && !s.drag.on && c.HoverAt(-1) {
				s.drawMenu()
			}
			return 0, true
		case wmLButtonDown:
			if c != nil {
				switch kind, i := s.layout.HitTest(lparamXY(lp)); kind {
				case "grip", "panel":
					s.dragStart()
				case "":
				default:
					s.apply(c.Click(kind, i))
				}
			}
			return 0, true
		case wmLButtonUp:
			if s.drag.on {
				s.dragEnd()
			}
			return 0, true
		case wmCaptureChanged:
			if s.drag.on {
				s.dragEnd()
			}
			return 0, true
		case wmRButtonDown:
			if c != nil {
				c.Close()
				s.sync()
			}
			return 0, true
		}
		return 0, false
	}
	if hwnd == s.wheelW && s.wheelW != 0 && m == wmMouseActivate {
		return maNoActivate, true
	}
	if hwnd != s.hwnd || s.hwnd == 0 {
		return 0, false
	}
	switch m {
	case wmShellOpen, wmShellToggle:
		s.openMenu(m == wmShellToggle)
	case wmShellClose:
		if c != nil {
			c.Close()
		}
		s.sync()
	case wmShellSync:
		s.sync()
	case wmShellWheel:
		s.openWheel(uint32(wp))
	case wmShellWSync:
		s.syncWheel()
	case wmShellSay:
		s.openSay()
	case wmShellRefresh:
		s.refresh()
	case wmShellToast:
		s.showToast()
	case wmShellTip:
		if s.trayAdded {
			d := s.nid(nifTip)
			pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&d)))
		}
	case wmShellBalloon:
		s.mu.Lock()
		b := s.balloon
		s.mu.Unlock()
		if s.trayAdded {
			d := s.nid(nifInfo)
			copyUTF16(d.SzInfoTitle[:], b[0])
			copyUTF16(d.SzInfo[:], b[1])
			d.DwInfoFlags = niifInfo
			pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&d)))
		}
	case wmShellQuit:
		s.removeTray()
		pDestroyWindow.Call(s.menuW)
		pDestroyWindow.Call(s.toastW)
		pDestroyWindow.Call(s.wheelW)
		pDestroyWindow.Call(s.hwnd)
		pPostQuitMessage.Call(0)
	case wmTimer:
		switch wp {
		case timerRefresh:
			s.refresh()
		case timerToast:
			pKillTimer.Call(s.hwnd, timerToast)
			pSetTimer.Call(s.hwnd, timerFade, 16, 0)
		case timerFade:
			s.toastAlpha -= 24
			if s.toastAlpha <= 0 {
				pKillTimer.Call(s.hwnd, timerFade)
				pShowWindow.Call(s.toastW, swHide)
			} else {
				blend := uint32(s.toastAlpha)<<16 | acSrcAlpha<<24
				pUpdateLayeredWindow.Call(s.toastW, 0, 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
			}
		case timerDrag:
			switch {
			case !s.drag.on:
				pKillTimer.Call(s.hwnd, timerDrag)
			case !leftDown(): // отпустили где-то за окном меню
				s.dragEnd()
			default:
				s.dragMove()
			}
		case timerWheel:
			s.tickWheel()
		case timerTray:
			s.addTray()
			if s.trayAdded {
				pKillTimer.Call(s.hwnd, timerTray)
			}
		}
	case wmTray:
		switch uint32(lp & 0xFFFF) {
		case wmLButtonUp, wmLButtonDblClk:
			if s.o.OnTray != nil {
				go s.o.OnTray("open")
			}
		case wmRButtonUp, wmContextMenu:
			s.trayMenu()
		}
	default:
		if s.taskbarMsg != 0 && uintptr(m) == s.taskbarMsg { // Проводник перезапустился — вернуть значок
			s.trayAdded = false
			s.addTray()
			return 0, true
		}
		return 0, false
	}
	return 0, true
}

// ---------- быстрое меню ----------

func (s *Shell) apply(r overlay.Result) {
	if r.Act != nil {
		s.queue(r.Act)
	}
	if r.Redraw || r.Close {
		s.sync()
	}
}

func (s *Shell) queue(f func()) {
	select {
	case s.acts <- f:
	default:
	}
}

// worker выполняет действия меню по порядку (звук, эффект) вне потоков окна и перехватчика.
func (s *Shell) worker() {
	for f := range s.acts {
		func() {
			defer func() {
				if e := recover(); e != nil {
					log.Printf("быстрое меню: %v", e)
				}
			}()
			f()
		}()
		s.post(wmShellRefresh)
	}
}

func (s *Shell) openMenu(toggle bool) {
	c := s.o.Menu
	if c == nil {
		return
	}
	if c.Visible() {
		if toggle {
			c.Close()
			s.sync()
		}
		return
	}
	if time.Since(time.Unix(0, s.closedAt.Load())) < 400*time.Millisecond {
		return // меню только что закрыла эта же горячая клавиша
	}
	if w := s.o.Wheel; w != nil && w.Visible() {
		w.Close()
		s.syncWheel()
	}
	c.Open()
	d := c.Data()
	s.placeOnMonitor(d.Position, d.Monitor)
	s.shown = false
	s.drawMenu()
	pSetTimer.Call(s.hwnd, timerRefresh, 250, 0)
}

// sync — показать меню в текущем состоянии или спрятать.
func (s *Shell) sync() {
	if c := s.o.Menu; c != nil && c.Visible() {
		s.drawMenu()
		return
	}
	s.hideMenu()
}

func (s *Shell) refresh() {
	c := s.o.Menu
	if c == nil || !c.Visible() {
		s.hideMenu()
		return
	}
	if c.Expired(time.Now()) {
		c.Close()
		s.hideMenu()
		return
	}
	if c.Refresh() {
		s.drawMenu()
	}
}

func (s *Shell) hideMenu() {
	if s.drag.on {
		s.dragEnd()
	}
	if s.shown {
		pShowWindow.Call(s.menuW, swHide)
	}
	s.shown, s.tracking, s.hand, s.moveCur = false, false, false, false
	pKillTimer.Call(s.hwnd, timerRefresh)
	s.mu.Lock()
	s.menuRect = image.Rectangle{}
	s.mu.Unlock()
}

// placeOnMonitor выбирает монитор (где игра, где курсор или основной) и масштаб.
func (s *Shell) placeOnMonitor(pos, monitor string) {
	var cur point32
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
	s.cursor = image.Pt(int(cur.X), int(cur.Y))
	fg, _, _ := pGetForegroundWindow.Call()
	var hmon uintptr
	switch {
	case pos == "cursor" || monitor == "cursor":
	case monitor == "primary": // основной монитор — тот, где точка (0, 0)
		hmon, _, _ = pMonitorFromPoint.Call(0, monitorNearest)
	case fg != 0:
		hmon, _, _ = pMonitorFromWindow.Call(fg, monitorNearest)
	}
	if hmon == 0 {
		hmon, _, _ = pMonitorFromPoint.Call(uintptr(uint32(cur.X))|uintptr(uint32(cur.Y))<<32, monitorNearest)
	}
	s.area, s.scale = monitorArea(hmon, fg)
}

// monitorArea — рабочая область монитора (весь монитор, если игра на весь экран) и масштаб.
func monitorArea(hmon, fg uintptr) (image.Rectangle, float64) {
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if hmon == 0 || func() bool { r, _, _ := pGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); return r == 0 }() {
		w, _, _ := pGetSystemMetrics.Call(0)
		h, _, _ := pGetSystemMetrics.Call(1)
		return image.Rect(0, 0, int(w), int(h)), 1
	}
	area := mi.Work.rect()
	if fg != 0 {
		var wr rect32
		pGetWindowRect.Call(fg, uintptr(unsafe.Pointer(&wr)))
		if mon := mi.Monitor.rect(); mon.In(wr.rect()) {
			area = mon
		}
	}
	dpi := uint32(0)
	if pGetDpiForMonitor.Find() == nil {
		var dx, dy uint32
		if r, _, _ := pGetDpiForMonitor.Call(hmon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 {
			dpi = dx
		}
	}
	if dpi == 0 {
		dc, _, _ := pGetDC.Call(0)
		d, _, _ := pGetDeviceCaps.Call(dc, logPixelsX)
		pReleaseDC.Call(0, dc)
		dpi = uint32(d)
	}
	if dpi == 0 {
		dpi = 96
	}
	return area, float64(dpi) / 96
}

func (s *Shell) drawMenu() {
	c := s.o.Menu
	if c == nil || s.o.R == nil || !c.Visible() {
		s.hideMenu()
		return
	}
	d := c.Data()
	v := c.View(s.scale)
	v.MaxW = s.area.Dx() // полоска ужимается под ширину экрана
	img, L := s.o.R.Menu(v)
	size := image.Pt(L.W, L.H)
	switch {
	case !s.shown:
		p := overlay.Placement{Position: d.Position, X: d.X, Y: d.Y, Cursor: s.cursor, Scale: s.scale}
		s.origin = overlay.Place(s.area, L.W, L.H, L.Panel, p)
		s.ax, s.ay = overlay.Align(p)
	case size != s.size: // другая вкладка или меньше строк — держим нужный край (у нижнего меню — низ)
		s.origin = overlay.Reanchor(s.origin, s.size, size, s.ax, s.ay)
	}
	s.origin = overlay.Clamp(s.area, s.origin, L.Panel)
	s.size = size
	s.layout = L
	if !updateLayered(s.menuW, img, s.origin, 255) {
		return
	}
	if !s.shown {
		pSetWindowPos.Call(s.menuW, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		s.shown = true
	}
	s.mu.Lock()
	s.menuRect = L.Panel.Add(s.origin)
	s.mu.Unlock()
}

// ---------- перетаскивание меню мышью ----------

func cursorPos() image.Point {
	var cur point32
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
	return image.Pt(int(cur.X), int(cur.Y))
}

func setCursor(id uintptr) {
	cur, _, _ := pLoadCursorW.Call(0, id)
	pSetCursor.Call(cur)
}

func (s *Shell) cursorID() uintptr {
	switch {
	case s.drag.on && s.drag.moved, s.moveCur:
		return idcSizeAll
	case s.hand:
		return idcHand
	}
	return idcArrow
}

// leftDown — зажата ли левая кнопка мыши (с учётом «поменять кнопки местами»).
func leftDown() bool {
	vk := uint32(0x01) // VK_LBUTTON
	if sw, _, _ := pGetSystemMetrics.Call(smSwapButton); sw != 0 {
		vk = 0x02
	}
	return IsKeyDown(vk)
}

func (s *Shell) dragStart() {
	s.drag = menuDrag{on: true, start: cursorPos(), orig: s.origin}
	pSetCapture.Call(s.menuW)
	pSetTimer.Call(s.hwnd, timerDrag, 15, 0)
}

func (s *Shell) dragMove() {
	d := cursorPos().Sub(s.drag.start)
	if !s.drag.moved {
		th := int(4*s.scale + 0.5)
		if d.X <= th && d.X >= -th && d.Y <= th && d.Y >= -th {
			return // ещё не тащат — просто щелчок
		}
		s.drag.moved = true
		setCursor(idcSizeAll)
	}
	o := overlay.Clamp(s.area, s.drag.orig.Add(d), s.layout.Panel)
	if o == s.origin {
		return
	}
	s.origin = o
	pSetWindowPos.Call(s.menuW, 0, uintptr(o.X), uintptr(o.Y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
	s.mu.Lock()
	s.menuRect = s.layout.Panel.Add(o)
	s.mu.Unlock()
	if c := s.o.Menu; c != nil {
		c.Touch()
	}
}

// dragEnd — отпустили: запомнить место (дальше меню будет появляться здесь).
func (s *Shell) dragEnd() {
	moved := s.drag.moved
	s.drag.on = false // до ReleaseCapture: он пришлёт WM_CAPTURECHANGED
	pKillTimer.Call(s.hwnd, timerDrag)
	pReleaseCapture.Call()
	if !moved {
		return
	}
	x, y := overlay.CustomXY(s.area, s.origin, s.layout.Panel)
	s.ax, s.ay = overlay.Align(overlay.Placement{Position: "custom", X: x, Y: y})
	if f := s.o.OnMenuMoved; f != nil {
		go f(x, y)
	}
}

// updateLayered показывает картинку (RGBA premultiplied → BGRA) в layered-окне.
func updateLayered(hwnd uintptr, img *image.RGBA, pos image.Point, alpha byte) bool {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 {
		return false
	}
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	mem, _, _ := pCreateCompatibleDC.Call(screen)
	if mem == 0 {
		return false
	}
	defer pDeleteDC.Call(mem)
	bi := bitmapInfo{H: bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}}
	bi.H.Size = uint32(unsafe.Sizeof(bi.H))
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return false
	}
	defer pDeleteObject.Call(bmp)
	old, _, _ := pSelectObject.Call(mem, bmp)
	defer pSelectObject.Call(mem, old)
	dst := unsafe.Slice((*byte)(bits), w*h*4)
	for y := 0; y < h; y++ {
		o := img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y)
		src := img.Pix[o : o+w*4]
		d := dst[y*w*4 : (y+1)*w*4]
		for i := 0; i < len(src); i += 4 {
			d[i], d[i+1], d[i+2], d[i+3] = src[i+2], src[i+1], src[i], src[i+3]
		}
	}
	pt := point32{int32(pos.X), int32(pos.Y)}
	sz := size32{int32(w), int32(h)}
	var zero point32
	blend := uint32(alpha)<<16 | acSrcAlpha<<24
	r, _, _ := pUpdateLayeredWindow.Call(hwnd, screen, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&sz)),
		mem, uintptr(unsafe.Pointer(&zero)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	return r != 0
}

// filterKey — из перехватчика клавиатуры (должен работать быстро). true — клавишу съели.
func (s *Shell) filterKey(vk, scan uint32, down bool) bool {
	s.mu.Lock()
	held := s.pressed[vk]
	if down {
		s.keyDown[vk] = true
	} else {
		delete(s.keyDown, vk)
		delete(s.pressed, vk)
	}
	s.mu.Unlock()
	if w := s.o.Wheel; w != nil && w.Visible() {
		return s.wheelKey(w, vk, down, held)
	}
	if !down {
		return held
	}
	c := s.o.Menu
	open := c != nil && c.Visible()
	// стрелки и PgUp/PgDn листают и при удержании, Backspace в строке поиска стирает
	repeat := vk >= overlay.VKPageUp && vk <= overlay.VKDown || open && vk == overlay.VKBack && c.Typing()
	if held && !repeat {
		return true // автоповтор съеденной клавиши
	}
	if !open {
		return false
	}
	if IsKeyDown(0x5B) || IsKeyDown(0x5C) { // сочетания с Win — системе (Win+Пробел меняет раскладку)
		return false
	}
	ctrl, alt := IsKeyDown(0x11), IsKeyDown(0x12)
	in := overlay.KeyInput{VK: vk, Ctrl: ctrl && !alt, Alt: alt && !ctrl}
	if c.Typing() {
		in.Text = keyText(vk, scan, IsKeyDown(0x10), ctrl && alt)
	}
	r := c.Input(in)
	if r.Act != nil {
		s.queue(r.Act)
	}
	if r.Redraw || r.Close {
		s.post(wmShellSync)
	}
	if r.Close && !r.Handled {
		s.closedAt.Store(time.Now().UnixNano())
	}
	if r.Handled {
		s.mu.Lock()
		s.pressed[vk] = true
		s.mu.Unlock()
	}
	return r.Handled
}

// mouseButton — кнопка мыши из сообщения перехватчика (коды VK: 1 левая, 2 правая, 4 средняя, 5/6 боковые)
// и отпускание ли это.
func mouseButton(m uintptr, data uint32) (vk uint32, up bool) {
	switch m {
	case wmLButtonDown, wmLButtonUp:
		return 0x01, m == wmLButtonUp
	case wmRButtonDown, wmRButtonUp:
		return 0x02, m == wmRButtonUp
	case wmMButtonDown, wmMButtonUp:
		return 0x04, m == wmMButtonUp
	case wmXButtonDown, wmXButtonUp:
		switch data >> 16 {
		case 1:
			return 0x05, m == wmXButtonUp
		case 2:
			return 0x06, m == wmXButtonUp
		}
	}
	return 0, false
}

// filterMouse — щелчок мимо меню закрывает его, колёсико над меню листает страницы; колесо звуков —
// выбор левой кнопкой, правая закрывает.
func (s *Shell) filterMouse(m uintptr, x, y int32, data uint32) bool {
	btn, up := mouseButton(m, data)
	if btn != 0 {
		s.mu.Lock()
		eaten := up && s.mouseEat[btn]
		if up {
			delete(s.mouseEat, btn)
			delete(s.keyDown, btn)
		} else {
			s.keyDown[btn] = true
		}
		s.mu.Unlock()
		if eaten {
			return true // нажатие съели — отпускание тоже
		}
	}
	if w := s.o.Wheel; w != nil && w.Visible() {
		return s.wheelMouse(w, btn, up, image.Pt(int(x), int(y)))
	}
	c := s.o.Menu
	if c == nil || !c.Visible() {
		return false
	}
	s.mu.Lock()
	in := image.Pt(int(x), int(y)).In(s.menuRect)
	s.mu.Unlock()
	switch m {
	case wmLButtonDown, wmRButtonDown, wmMButtonDown, wmXButtonDown:
		if !in {
			c.Close()
			s.closedAt.Store(time.Now().UnixNano())
			s.post(wmShellSync)
		}
	case wmMouseWheel:
		if in {
			r := c.Wheel(int(int16(data >> 16)))
			if r.Redraw {
				s.post(wmShellSync)
			}
			return r.Handled
		}
	}
	return false
}

// ---------- колесо звуков ----------

// wheelKey — клавиатура, пока открыто колесо (из перехватчика).
func (s *Shell) wheelKey(w *overlay.WheelCtl, vk uint32, down, held bool) bool {
	own := vk == w.VK()
	r := w.Key(vk, down, time.Now())
	s.applyWheel(r)
	if !down {
		// отпускание клавиши колеса отдаём игре: её нажатие съела горячая клавиша, а состояние
		// клавиатуры у системы и игры должно сойтись
		return held
	}
	if r.Handled && (!own || r.Close) { // съели нажатие — съедим и отпускание (автоповтор клавиши колеса — нет)
		s.mu.Lock()
		s.pressed[vk] = true
		s.mu.Unlock()
	}
	return r.Handled || held
}

// wheelMouse — кнопки мыши, пока открыто колесо: левая выбирает, правая закрывает,
// отпускание кнопки колеса (если колесо на кнопке мыши) — выбор.
func (s *Shell) wheelMouse(w *overlay.WheelCtl, btn uint32, up bool, p image.Point) bool {
	if btn == 0 {
		return false // колёсико и прочее — игре
	}
	now := time.Now()
	var r overlay.Result
	switch {
	case btn == w.VK():
		r = w.Key(btn, !up, now)
	case up:
		return false
	case btn == 0x01:
		r = w.Click(p, now)
	case btn == 0x02:
		w.Close()
		r = overlay.Result{Handled: true, Close: true}
	default:
		return false
	}
	s.applyWheel(r)
	if r.Handled && !up {
		s.mu.Lock()
		s.mouseEat[btn] = true
		s.mu.Unlock()
	}
	return r.Handled && !(btn == w.VK() && up)
}

// applyWheel — из потока перехватчика: действие в очередь, окно перерисует поток окон.
func (s *Shell) applyWheel(r overlay.Result) {
	if r.Act != nil {
		s.queue(r.Act)
	}
	if r.Redraw || r.Close {
		s.post(wmShellWSync)
	}
}

func (s *Shell) isDown(vk uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keyDown[vk]
}

// openWheel — показать колесо у курсора (или по центру экрана с курсором).
func (s *Shell) openWheel(vk uint32) {
	w := s.o.Wheel
	if w == nil || s.o.R == nil {
		return
	}
	if w.Visible() {
		if vk == 0 || w.Sticky() { // ещё раз из трея или интерфейса — закрыть
			w.Close()
			s.syncWheel()
		}
		return
	}
	if c := s.o.Menu; c != nil && c.Visible() {
		c.Close()
		s.sync()
	}
	d := w.Load()
	cur := cursorPos()
	hmon, _, _ := pMonitorFromPoint.Call(uintptr(uint32(cur.X))|uintptr(uint32(cur.Y))<<32, monitorNearest)
	fg, _, _ := pGetForegroundWindow.Call()
	area, dpi := monitorArea(hmon, fg)
	sc := d.Scale
	if sc <= 0 {
		sc = 1
	}
	scale := dpi * sc
	side, rad := overlay.WheelSize(scale), overlay.WheelRadius(scale)
	center := cur
	if d.Center == "screen" {
		center = image.Pt((area.Min.X+area.Max.X)/2, (area.Min.Y+area.Max.Y)/2)
	}
	fit := func(v, lo, hi int) int { // не выходить за край экрана
		if hi < lo {
			return (lo + hi) / 2
		}
		return min(max(v, lo), hi)
	}
	center.X = fit(center.X, area.Min.X+rad, area.Max.X-rad)
	center.Y = fit(center.Y, area.Min.Y+rad, area.Max.Y-rad)
	s.wheel.pos = center.Sub(image.Pt(side/2, side/2))
	s.wheel.rev = -1
	w.Open(d, vk, vk != 0 && s.isDown(vk), cur, center, scale, time.Now())
	s.drawWheel()
	pSetTimer.Call(s.hwnd, timerWheel, 15, 0)
}

func (s *Shell) syncWheel() {
	if w := s.o.Wheel; w != nil && w.Visible() {
		s.drawWheel()
		return
	}
	s.hideWheel()
}

func (s *Shell) tickWheel() {
	w := s.o.Wheel
	if w == nil || !w.Visible() {
		s.hideWheel()
		return
	}
	vk := w.VK()
	r := w.Tick(cursorPos(), vk != 0 && s.isDown(vk), time.Now())
	if r.Act != nil {
		s.queue(r.Act)
	}
	if r.Redraw || r.Close {
		s.syncWheel()
	}
}

func (s *Shell) drawWheel() {
	w := s.o.Wheel
	if w == nil || s.o.R == nil || !w.Visible() {
		s.hideWheel()
		return
	}
	v := w.View()
	ww := &s.wheel
	if ww.base == nil || v.Rev != ww.rev {
		ww.base, ww.rev = s.o.R.WheelBase(v), v.Rev
	}
	if ww.buf == nil || ww.buf.Rect != ww.base.Rect {
		ww.buf = image.NewRGBA(ww.base.Rect)
	}
	copy(ww.buf.Pix, ww.base.Pix)
	overlay.WheelPointer(ww.buf, v)
	if !updateLayered(s.wheelW, ww.buf, ww.pos, 255) {
		return
	}
	if !ww.shown {
		pSetWindowPos.Call(s.wheelW, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
		ww.shown = true
	}
}

func (s *Shell) hideWheel() {
	pKillTimer.Call(s.hwnd, timerWheel)
	if s.wheel.shown {
		pShowWindow.Call(s.wheelW, swHide)
	}
	s.wheel = wheelWin{}
}

// ---------- уведомления ----------

func ownForeground() bool {
	fg, _, _ := pGetForegroundWindow.Call()
	if fg == 0 {
		return false
	}
	var pid uint32
	pGetWindowThreadProcessId.Call(fg, uintptr(unsafe.Pointer(&pid)))
	me, _, _ := pGetCurrentProcessId.Call()
	return pid == uint32(me)
}

func (s *Shell) showToast() {
	s.mu.Lock()
	v := s.toastQ
	s.toastQ = nil
	s.mu.Unlock()
	if v == nil || s.o.R == nil {
		return
	}
	if ownForeground() { // открыто окно VoiceBox — там и так всё видно
		return
	}
	fg, _, _ := pGetForegroundWindow.Call()
	var hmon uintptr
	if fg != 0 {
		hmon, _, _ = pMonitorFromWindow.Call(fg, monitorNearest)
	}
	area, scale := monitorArea(hmon, fg)
	v.Scale = scale
	img := s.o.R.Toast(*v)
	w, h := img.Rect.Dx(), img.Rect.Dy()
	pos := image.Pt(area.Min.X+(area.Dx()-w)/2, area.Min.Y+area.Dy()*11/100)
	if c := s.o.Menu; c != nil && c.Visible() { // не закрывать меню: под ним, над ним или внизу экрана
		s.mu.Lock()
		mr := s.menuRect
		s.mu.Unlock()
		if image.Rect(pos.X, pos.Y, pos.X+w, pos.Y+h).Overlaps(mr) {
			switch below, above := mr.Max.Y-int(8*scale), mr.Min.Y-h+int(8*scale); {
			case below+h <= area.Max.Y:
				pos.Y = below
			case above >= area.Min.Y:
				pos.Y = above
			default:
				pos.Y = area.Max.Y - h - area.Dy()*22/100
			}
		}
	}
	pKillTimer.Call(s.hwnd, timerFade)
	s.toastAlpha = 255
	if !updateLayered(s.toastW, img, pos, 255) {
		return
	}
	pSetWindowPos.Call(s.toastW, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
	n := utf8.RuneCountInString(v.Text + v.Sub)
	ms := min(max(1500+40*n, 1800), 4500)
	pSetTimer.Call(s.hwnd, timerToast, uintptr(ms), 0)
}

// ---------- значок в трее ----------

func (s *Shell) nid(flags uint32) notifyIconData {
	d := notifyIconData{HWnd: s.hwnd, UID: 1, UFlags: flags, UCallbackMessage: wmTray, HIcon: s.icon}
	d.CbSize = uint32(unsafe.Sizeof(d))
	s.mu.Lock()
	tip := s.tip
	s.mu.Unlock()
	if tip == "" {
		tip = "VoiceBox"
	}
	copyUTF16(d.SzTip[:], tip)
	return d
}

func (s *Shell) addTray() {
	if s.trayAdded {
		return
	}
	d := s.nid(nifMessage | nifIcon | nifTip)
	r, _, _ := pShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&d)))
	if r == 0 { // значок уже есть (повторный вызов) — обновим
		r, _, _ = pShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&d)))
	}
	s.trayAdded = r != 0
}

func (s *Shell) removeTray() {
	if !s.trayAdded {
		return
	}
	d := s.nid(0)
	pShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
	s.trayAdded = false
}

func (s *Shell) buildMenu(h uintptr, items []TrayItem) {
	for _, it := range items {
		title := strings.ReplaceAll(it.Title, "&", "&&")
		switch {
		case it.Sep:
			pAppendMenuW.Call(h, mfSeparator, 0, 0)
		case len(it.Sub) > 0:
			sub, _, _ := pCreatePopupMenu.Call()
			s.buildMenu(sub, it.Sub)
			f := uintptr(mfPopup)
			if it.Disabled {
				f |= mfGrayed
			}
			pAppendMenuW.Call(h, f, sub, uintptr(unsafe.Pointer(wstr(title))))
		default:
			s.trayIDs = append(s.trayIDs, it.ID)
			id := uintptr(len(s.trayIDs))
			f := uintptr(mfString)
			if it.Checked {
				f |= mfChecked
			}
			if it.Disabled {
				f |= mfGrayed
			}
			pAppendMenuW.Call(h, f, id, uintptr(unsafe.Pointer(wstr(title))))
			if it.Default {
				pSetMenuDefaultItem.Call(h, id, 0)
			}
		}
	}
}

func (s *Shell) trayMenu() {
	if s.o.TrayMenu == nil {
		return
	}
	items := s.o.TrayMenu()
	h, _, _ := pCreatePopupMenu.Call()
	if h == 0 {
		return
	}
	s.trayIDs = s.trayIDs[:0]
	s.buildMenu(h, items)
	var pt point32
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	pSetForegroundWindow.Call(s.hwnd) // иначе меню не закроется щелчком мимо
	cmd, _, _ := pTrackPopupMenu.Call(h, tpmRightButton|tpmReturnCmd|tpmNoNotify, uintptr(pt.X), uintptr(pt.Y), 0, s.hwnd, 0)
	pPostMessageW.Call(s.hwnd, wmNull, 0, 0)
	pDestroyMenu.Call(h)
	if cmd > 0 && int(cmd) <= len(s.trayIDs) && s.o.OnTray != nil {
		id := s.trayIDs[cmd-1]
		go s.o.OnTray(id)
	}
}

// ---------- для приложения (из любых потоков) ----------

// ToggleMenu — открыть быстрое меню или закрыть, если открыто.
func (s *Shell) ToggleMenu() {
	if s != nil {
		s.post(wmShellToggle)
	}
}

func (s *Shell) OpenMenu() {
	if s != nil {
		s.post(wmShellOpen)
	}
}

func (s *Shell) CloseMenu() {
	if s != nil {
		s.post(wmShellClose)
	}
}

// RefreshMenu — перечитать пункты (например, после изменения настроек).
func (s *Shell) RefreshMenu() {
	if s != nil {
		s.post(wmShellRefresh)
	}
}

// OpenWheel показывает колесо звуков: vk — клавиша, которую держат (отпустили — выбор), 0 — выбор щелчком.
func (s *Shell) OpenWheel(vk uint32) {
	if s != nil && s.hwnd != 0 && s.o.Wheel != nil {
		pPostMessageW.Call(s.hwnd, wmShellWheel, uintptr(vk), 0)
	}
}

// OpenSay — быстрое меню сразу со строкой поиска: найти звук или сказать фразу команде.
func (s *Shell) OpenSay() {
	if s != nil {
		s.post(wmShellSay)
	}
}

func (s *Shell) openSay() {
	c := s.o.Menu
	if c == nil {
		return
	}
	if !c.Visible() {
		s.openMenu(false)
	}
	if c.Visible() {
		c.StartTyping()
		s.drawMenu()
	}
}

// MenuVisible — открыто ли меню.
func (s *Shell) MenuVisible() bool { return s != nil && s.o.Menu != nil && s.o.Menu.Visible() }

// Toast — уведомление поверх игры (новое заменяет текущее).
func (s *Shell) Toast(v overlay.ToastView) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.toastQ = &v
	s.mu.Unlock()
	s.post(wmShellToast)
}

// SetTooltip — подсказка значка в трее.
func (s *Shell) SetTooltip(t string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	same := s.tip == t
	s.tip = t
	s.mu.Unlock()
	if !same {
		s.post(wmShellTip)
	}
}

// Balloon — системное уведомление от значка в трее.
func (s *Shell) Balloon(title, text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.balloon = [2]string{title, text}
	s.mu.Unlock()
	s.post(wmShellBalloon)
}

// Close убирает значок и окна.
func (s *Shell) Close() {
	if s == nil {
		return
	}
	SetHookFilter(nil, nil)
	s.post(wmShellQuit)
	select {
	case <-s.done:
	case <-time.After(time.Second):
	}
}
