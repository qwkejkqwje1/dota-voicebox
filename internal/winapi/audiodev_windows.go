//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Устройства звука Windows через Core Audio (IMMDeviceEnumerator) и IPolicyConfig
// (недокументированный, но стабильный с Windows 7 интерфейс, которым пользуется «Панель звука»).

var (
	ole32               = syscall.NewLazyDLL("ole32.dll")
	pCoCreateInstance   = ole32.NewProc("CoCreateInstance")
	pPropVariantClear   = ole32.NewProc("PropVariantClear")
	clsidMMDeviceEnum   = windows.GUID{Data1: 0xBCDE0395, Data2: 0xE52F, Data3: 0x467C, Data4: [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnum    = windows.GUID{Data1: 0xA95664D2, Data2: 0x9614, Data3: 0x4F35, Data4: [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	clsidPolicyConfig   = windows.GUID{Data1: 0x870af99c, Data2: 0x171d, Data3: 0x4f9e, Data4: [8]byte{0xaf, 0x0d, 0xe6, 0x3d, 0xf4, 0x0c, 0x2b, 0xc9}}
	iidIPolicyConfig    = windows.GUID{Data1: 0xf8679f50, Data2: 0x850a, Data3: 0x41cf, Data4: [8]byte{0x9c, 0x72, 0x43, 0x0f, 0x29, 0x02, 0x90, 0xc8}}
	pkeyFriendlyName    = propKey{windows.GUID{Data1: 0xa45c254e, Data2: 0xdf1c, Data3: 0x4efd, Data4: [8]byte{0x80, 0x20, 0x67, 0xd1, 0x46, 0xa8, 0x50, 0xe0}}, 14}
	pkeyEngineDevFormat = propKey{windows.GUID{Data1: 0xf19f064d, Data2: 0x082c, Data3: 0x4e27, Data4: [8]byte{0xbc, 0x73, 0x68, 0x82, 0xa1, 0xbb, 0x8e, 0x4c}}, 0}
)

type propKey struct {
	fmtid windows.GUID
	pid   uint32
}

type propVariant struct {
	vt         uint16
	_, _, _    uint16
	val1, val2 unsafe.Pointer
}

type comObj struct{ vtbl *[32]uintptr }

func (o *comObj) call(i int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(o.vtbl[i], append([]uintptr{uintptr(unsafe.Pointer(o))}, args...)...)
	return r
}
func (o *comObj) release() { o.call(2) }

func hr(r uintptr, what string) error {
	if int32(r) < 0 {
		return fmt.Errorf("%s: 0x%08X", what, uint32(r))
	}
	return nil
}

const (
	eRender, eCapture           = 0, 1
	deviceStateActive           = 1
	roleConsole, roleMultimedia = 0, 1
	roleCommunications          = 2
)

// withCOM выполняет f в потоке с инициализированным COM.
func withCOM(f func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err == nil {
		defer windows.CoUninitialize()
	}
	return f()
}

func create(clsid, iid *windows.GUID) (*comObj, error) {
	var o *comObj
	r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, 0x17 /*CLSCTX_ALL*/, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&o)))
	if err := hr(r, "CoCreateInstance"); err != nil {
		return nil, err
	}
	return o, nil
}

// AudioEndpoint — активное устройство звука Windows.
type AudioEndpoint struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Capture     bool   `json:"capture"`
	Default     bool   `json:"default"`      // «устройство по умолчанию»
	DefaultComm bool   `json:"default_comm"` // «устройство связи по умолчанию»
	Rate        int    `json:"rate"`         // частота в общем режиме (Гц), 0 — неизвестно
	Bits        int    `json:"bits"`
}

func devID(d *comObj) string {
	var p *uint16
	if d.call(5, uintptr(unsafe.Pointer(&p))) != 0 || p == nil {
		return ""
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(p))
	return windows.UTF16PtrToString(p)
}

func defaultID(en *comObj, flow, role int) string {
	var d *comObj
	if en.call(4, uintptr(flow), uintptr(role), uintptr(unsafe.Pointer(&d))) != 0 || d == nil {
		return ""
	}
	defer d.release()
	return devID(d)
}

func readProps(d *comObj, e *AudioEndpoint) {
	var st *comObj
	if d.call(4, 0 /*STGM_READ*/, uintptr(unsafe.Pointer(&st))) != 0 || st == nil {
		return
	}
	defer st.release()
	var v propVariant
	if st.call(5, uintptr(unsafe.Pointer(&pkeyFriendlyName)), uintptr(unsafe.Pointer(&v))) == 0 && v.vt == 31 /*VT_LPWSTR*/ && v.val1 != nil {
		e.Name = windows.UTF16PtrToString((*uint16)(v.val1))
	}
	pPropVariantClear.Call(uintptr(unsafe.Pointer(&v)))
	v = propVariant{}
	if st.call(5, uintptr(unsafe.Pointer(&pkeyEngineDevFormat)), uintptr(unsafe.Pointer(&v))) == 0 && v.vt == 65 /*VT_BLOB*/ && uint32(uintptr(v.val1)) >= 16 && v.val2 != nil {
		wf := (*waveFormatEx)(v.val2)
		e.Rate, e.Bits = int(wf.SamplesPerSec), int(wf.BitsPerSample)
	}
	pPropVariantClear.Call(uintptr(unsafe.Pointer(&v)))
}

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

// AudioEndpoints — все активные устройства воспроизведения и записи.
func AudioEndpoints() ([]AudioEndpoint, error) {
	var out []AudioEndpoint
	err := withCOM(func() error {
		en, err := create(&clsidMMDeviceEnum, &iidIMMDeviceEnum)
		if err != nil {
			return err
		}
		defer en.release()
		for _, flow := range []int{eRender, eCapture} {
			def, comm := defaultID(en, flow, roleConsole), defaultID(en, flow, roleCommunications)
			var col *comObj
			if err := hr(en.call(3, uintptr(flow), deviceStateActive, uintptr(unsafe.Pointer(&col))), "EnumAudioEndpoints"); err != nil {
				return err
			}
			var n uint32
			col.call(3, uintptr(unsafe.Pointer(&n)))
			for i := uint32(0); i < n; i++ {
				var d *comObj
				if col.call(4, uintptr(i), uintptr(unsafe.Pointer(&d))) != 0 || d == nil {
					continue
				}
				e := AudioEndpoint{ID: devID(d), Capture: flow == eCapture}
				e.Default, e.DefaultComm = e.ID == def, e.ID == comm
				readProps(d, &e)
				d.release()
				out = append(out, e)
			}
			col.release()
		}
		return nil
	})
	return out, err
}

// SetDefaultAudio делает устройство устройством по умолчанию (для всех ролей).
func SetDefaultAudio(id string) error {
	return withCOM(func() error {
		pc, err := create(&clsidPolicyConfig, &iidIPolicyConfig)
		if err != nil {
			return err
		}
		defer pc.release()
		p, _ := windows.UTF16PtrFromString(id)
		for _, role := range []int{roleConsole, roleMultimedia, roleCommunications} {
			if err := hr(pc.call(13, uintptr(unsafe.Pointer(p)), uintptr(role)), "SetDefaultEndpoint"); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetEndpointRate меняет частоту общего режима устройства (как «Свойства → Дополнительно»).
// Обычно требует прав администратора; при ошибке — ручной шаг через OpenSoundPanel.
func SetEndpointRate(id string, rate int) error {
	return withCOM(func() error {
		pc, err := create(&clsidPolicyConfig, &iidIPolicyConfig)
		if err != nil {
			return err
		}
		defer pc.release()
		p, _ := windows.UTF16PtrFromString(id)
		var cur *waveFormatEx
		if err := hr(pc.call(4, uintptr(unsafe.Pointer(p)), 0, uintptr(unsafe.Pointer(&cur))), "GetDeviceFormat"); err != nil {
			return err
		}
		if cur == nil {
			return errors.New("формат устройства неизвестен")
		}
		defer windows.CoTaskMemFree(unsafe.Pointer(cur))
		size := 18 + int(cur.CbSize)
		buf := make([]byte, size)
		copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(cur)), size))
		wf := (*waveFormatEx)(unsafe.Pointer(&buf[0]))
		wf.SamplesPerSec = uint32(rate)
		wf.AvgBytesPerSec = uint32(rate) * uint32(wf.BlockAlign)
		return hr(pc.call(6, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(wf)), uintptr(unsafe.Pointer(wf))), "SetDeviceFormat")
	})
}

// OpenSoundPanel открывает классическую «Панель звука» (вкладка: 0 — воспроизведение, 1 — запись).
func OpenSoundPanel(tab int) error {
	return exec.Command("rundll32.exe", "shell32.dll,Control_RunDLL", "mmsys.cpl,,"+fmt.Sprint(tab)).Start()
}

var (
	iidAudioSessionManager2 = windows.GUID{Data1: 0x77AA99A0, Data2: 0x1BD6, Data3: 0x484F, Data4: [8]byte{0x8B, 0xC7, 0x2C, 0x65, 0x4C, 0x9A, 0x9B, 0x6F}}
	iidAudioSessionControl2 = windows.GUID{Data1: 0xbfb7ff88, Data2: 0x7239, Data3: 0x4fc9, Data4: [8]byte{0x8f, 0xa2, 0x07, 0xc9, 0x50, 0xbe, 0x9c, 0x6d}}
)

// AppSession — программа, которая открыла устройство записи (например Discord → CABLE Output).
type AppSession struct {
	Device  string `json:"device"`
	Process string `json:"process"`
	PID     uint32 `json:"pid"`
	Active  bool   `json:"active"` // сейчас идёт захват
}

func procName(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 520)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return ""
	}
	p := windows.UTF16ToString(buf[:n])
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		p = p[i+1:]
	}
	return p
}

// CaptureSessions — какие программы пишут с каких микрофонов/кабелей.
func CaptureSessions() ([]AppSession, error) {
	var out []AppSession
	err := withCOM(func() error {
		en, err := create(&clsidMMDeviceEnum, &iidIMMDeviceEnum)
		if err != nil {
			return err
		}
		defer en.release()
		var col *comObj
		if err := hr(en.call(3, eCapture, deviceStateActive, uintptr(unsafe.Pointer(&col))), "EnumAudioEndpoints"); err != nil {
			return err
		}
		defer col.release()
		var n uint32
		col.call(3, uintptr(unsafe.Pointer(&n)))
		for i := uint32(0); i < n; i++ {
			var d *comObj
			if col.call(4, uintptr(i), uintptr(unsafe.Pointer(&d))) != 0 || d == nil {
				continue
			}
			var e AudioEndpoint
			readProps(d, &e)
			var mgr *comObj
			if d.call(3, uintptr(unsafe.Pointer(&iidAudioSessionManager2)), 0x17, 0, uintptr(unsafe.Pointer(&mgr))) == 0 && mgr != nil {
				var se *comObj
				if mgr.call(5, uintptr(unsafe.Pointer(&se))) == 0 && se != nil {
					var cnt int32
					se.call(3, uintptr(unsafe.Pointer(&cnt)))
					for k := int32(0); k < cnt; k++ {
						var sc *comObj
						if se.call(4, uintptr(k), uintptr(unsafe.Pointer(&sc))) != 0 || sc == nil {
							continue
						}
						var sc2 *comObj
						if sc.call(0, uintptr(unsafe.Pointer(&iidAudioSessionControl2)), uintptr(unsafe.Pointer(&sc2))) == 0 && sc2 != nil {
							var pid, state uint32
							sc2.call(14, uintptr(unsafe.Pointer(&pid)))
							sc2.call(3, uintptr(unsafe.Pointer(&state)))
							if pid != 0 && pid != uint32(windows.GetCurrentProcessId()) {
								out = append(out, AppSession{Device: e.Name, Process: procName(pid), PID: pid, Active: state == 1})
							}
							sc2.release()
						}
						sc.release()
					}
					se.release()
				}
				mgr.release()
			}
			d.release()
		}
		return nil
	})
	return out, err
}
