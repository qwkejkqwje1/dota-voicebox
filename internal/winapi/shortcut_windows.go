//go:build windows

package winapi

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
)

// Ярлыки portable-программы: меню «Пуск» и рабочий стол (IShellLinkW + IPersistFile),
// а также «самолечение» — программу перенесли в другую папку, и автозапуск с ярлыками
// указывают на путь, которого больше нет.

const shortcutName = "Dota VoiceBox.lnk"

var (
	clsidShellLink  = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIShellLinkW  = windows.GUID{Data1: 0x000214F9, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidIPersistFile = windows.GUID{Data1: 0x0000010B, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// методы IShellLinkW и IPersistFile (номера в таблице виртуальных функций)
const (
	slGetPath             = 3
	slSetDescription      = 7
	slSetWorkingDirectory = 9
	slSetIconLocation     = 17
	slSetPath             = 20
	pfLoad                = 5
	pfSave                = 6
)

func shortcutPath(where string) (string, error) {
	var id *windows.KNOWNFOLDERID
	switch where {
	case "start":
		id = windows.FOLDERID_Programs
	case "desktop":
		id = windows.FOLDERID_Desktop
	default:
		return "", fmt.Errorf("неизвестное место для ярлыка %q", where)
	}
	dir, err := windows.KnownFolderPath(id, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, shortcutName), nil
}

// HasShortcut — есть ли ярлык VoiceBox ("start" — меню «Пуск», "desktop" — рабочий стол).
func HasShortcut(where string) bool {
	p, err := shortcutPath(where)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// SetShortcut создаёт ярлык на текущий exe или удаляет его.
func SetShortcut(where string, on bool) error {
	p, err := shortcutPath(where)
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return writeShortcut(p, exe)
}

func shellLink(f func(sl, pf *comObj) error) error {
	return withCOM(func() error {
		sl, err := create(&clsidShellLink, &iidIShellLinkW)
		if err != nil {
			return err
		}
		defer sl.release()
		var pf *comObj
		if err := hr(sl.call(0, uintptr(unsafe.Pointer(&iidIPersistFile)), uintptr(unsafe.Pointer(&pf))), "IPersistFile"); err != nil {
			return err
		}
		defer pf.release()
		return f(sl, pf)
	})
}

func writeShortcut(lnk, exe string) error {
	path, _ := syscall.UTF16PtrFromString(exe)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	desc, _ := syscall.UTF16PtrFromString(i18n.T("VoiceBox — звуки и эффекты голоса для Discord и игр"))
	name, _ := syscall.UTF16PtrFromString(lnk)
	err := shellLink(func(sl, pf *comObj) error {
		if err := hr(sl.call(slSetPath, uintptr(unsafe.Pointer(path))), "SetPath"); err != nil {
			return err
		}
		sl.call(slSetWorkingDirectory, uintptr(unsafe.Pointer(dir)))
		sl.call(slSetDescription, uintptr(unsafe.Pointer(desc)))
		sl.call(slSetIconLocation, uintptr(unsafe.Pointer(path)), 0)
		return hr(pf.call(pfSave, uintptr(unsafe.Pointer(name)), 1), "сохранение ярлыка")
	})
	runtime.KeepAlive(path)
	runtime.KeepAlive(dir)
	runtime.KeepAlive(desc)
	runtime.KeepAlive(name)
	return err
}

func shortcutTarget(lnk string) (string, error) {
	name, _ := syscall.UTF16PtrFromString(lnk)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	err := shellLink(func(sl, pf *comObj) error {
		if err := hr(pf.call(pfLoad, uintptr(unsafe.Pointer(name)), 0 /*STGM_READ*/), "чтение ярлыка"); err != nil {
			return err
		}
		return hr(sl.call(slGetPath, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0), "GetPath")
	})
	runtime.KeepAlive(name)
	return windows.UTF16ToString(buf), err
}

// HealShortcuts — exe перенесли или переименовали: автозапуск и ярлыки, которые указывают
// на несуществующий файл, переписываются на текущий. Чужие рабочие копии (старый путь
// существует) не трогаем. Возвращает, что исправлено.
func HealShortcuts() []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	gone := func(p string) bool {
		if p == "" || strings.EqualFold(filepath.Clean(p), filepath.Clean(exe)) {
			return false
		}
		_, err := os.Stat(p)
		return errors.Is(err, os.ErrNotExist)
	}
	var fixed []string
	if k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE); err == nil {
		v, _, err := k.GetStringValue(runName)
		k.Close()
		if err == nil && gone(runTarget(v)) && SetAutostart(true) == nil {
			fixed = append(fixed, "автозапуск")
		}
	}
	for _, w := range []string{"start", "desktop"} {
		p, err := shortcutPath(w)
		if err != nil || !HasShortcut(w) {
			continue
		}
		if t, err := shortcutTarget(p); err == nil && gone(t) && writeShortcut(p, exe) == nil {
			fixed = append(fixed, map[string]string{"start": "ярлык в «Пуске»", "desktop": "ярлык на рабочем столе"}[w])
		}
	}
	return fixed
}
