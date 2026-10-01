//go:build windows

package winapi

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	shell32        = syscall.NewLazyDLL("shell32.dll")
	pShellExecuteW = shell32.NewProc("ShellExecuteW")
	pMessageBoxW   = user32.NewProc("MessageBoxW")
)

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

// SteamRunning — запущен ли steam.exe.
func SteamRunning() bool {
	out, _ := hidden(exec.Command("tasklist", "/FI", "IMAGENAME eq steam.exe", "/NH")).Output()
	return strings.Contains(strings.ToLower(string(out)), "steam.exe")
}

// DotaRunning — запущена ли Dota 2.
func DotaRunning() bool {
	out, _ := hidden(exec.Command("tasklist", "/FI", "IMAGENAME eq dota2.exe", "/NH")).Output()
	return strings.Contains(strings.ToLower(string(out)), "dota2.exe")
}

// ShutdownSteam корректно закрывает Steam и ждёт до 40 секунд.
func ShutdownSteam(steamRoot string) error {
	if !SteamRunning() {
		return nil
	}
	hidden(exec.Command(filepath.Join(steamRoot, "steam.exe"), "-shutdown")).Start()
	for i := 0; i < 80; i++ {
		time.Sleep(500 * time.Millisecond)
		if !SteamRunning() {
			time.Sleep(time.Second) // дать Steam дописать конфиги
			return nil
		}
	}
	return errors.New("Steam не закрылся за 40 секунд")
}

func StartSteam(steamRoot string) error {
	return exec.Command(filepath.Join(steamRoot, "steam.exe")).Start()
}

// RunElevated запускает программу с правами администратора (запрос UAC).
func RunElevated(exe, args string) error {
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(args)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(exe))
	r, _, _ := pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)), uintptr(unsafe.Pointer(dir)), 1)
	if r <= 32 {
		return errors.New("установка отменена или не запущена")
	}
	return nil
}

// OpenURL открывает ссылку или папку в системе.
func OpenURL(u string) error {
	return hidden(exec.Command("rundll32", "url.dll,FileProtocolHandler", u)).Start()
}

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const runName = "DotaVoiceBox"

func Autostart() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(runName)
	return err == nil
}

func SetAutostart(on bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !on {
		err := k.DeleteValue(runName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, _ := os.Executable()
	return k.SetStringValue(runName, `"`+exe+`" -background`)
}

// Alert показывает системное окно с сообщением (когда интерфейс недоступен).
func Alert(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}

// SetHighPriority — приоритет «выше среднего», чтобы звук не заикался во время игры.
func SetHighPriority() {
	windows.SetPriorityClass(windows.CurrentProcess(), windows.ABOVE_NORMAL_PRIORITY_CLASS)
}
