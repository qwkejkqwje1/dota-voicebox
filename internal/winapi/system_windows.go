//go:build windows

package winapi

import (
	"errors"
	"fmt"
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

	pAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

// AllowForeground разрешает другой программе (уже запущенной копии VoiceBox) вывести окно вперёд.
func AllowForeground() { pAllowSetForegroundWindow.Call(^uintptr(0)) } // ASFW_ANY

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

var (
	pShellExecuteExW     = shell32.NewProc("ShellExecuteExW")
	pWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	pGetExitCodeProcess  = kernel32.NewProc("GetExitCodeProcess")
	pCloseHandle         = kernel32.NewProc("CloseHandle")
)

// shellExecuteInfo — SHELLEXECUTEINFOW (раскладка полей как в C, 112 байт на x64).
type shellExecuteInfo struct {
	cbSize    uint32
	fMask     uint32
	hwnd      uintptr
	verb      *uint16
	file      *uint16
	params    *uint16
	dir       *uint16
	nShow     int32
	hInstApp  uintptr
	idList    uintptr
	class     *uint16
	hkeyClass uintptr
	hotKey    uint32
	hIcon     uintptr
	hProcess  uintptr
}

// runElevatedWait запускает программу с правами администратора (запрос UAC) без окна и ждёт завершения.
func runElevatedWait(file, args string, timeout time.Duration) error {
	const seeMaskNoCloseProcess, seeMaskNoAsync = 0x40, 0x100
	verb, _ := syscall.UTF16PtrFromString("runas")
	f, _ := syscall.UTF16PtrFromString(file)
	p, _ := syscall.UTF16PtrFromString(args)
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync, verb: verb, file: f, params: p, nShow: 0}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, e := pShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(e, windows.ERROR_CANCELLED) {
			return errors.New("отменено: в окне Windows не дали разрешение")
		}
		return e
	}
	if info.hProcess == 0 {
		return nil
	}
	defer pCloseHandle.Call(info.hProcess)
	if r, _, _ := pWaitForSingleObject.Call(info.hProcess, uintptr(timeout.Milliseconds())); r != 0 {
		return errors.New("команда не завершилась вовремя")
	}
	var code uint32
	pGetExitCodeProcess.Call(info.hProcess, uintptr(unsafe.Pointer(&code)))
	if code != 0 {
		return fmt.Errorf("netsh завершился с кодом %d", code)
	}
	return nil
}

// FirewallRuleName — имя правила брандмауэра, которое добавляет VoiceBox (и только его он и трогает).
const FirewallRuleName = "VoiceBox Remote"

// AllowFirewall добавляет в брандмауэр Windows узкое разрешающее правило для пульта с телефона:
// только эта программа, только TCP-порт пульта, только домашние (частные) сети и только устройства
// из той же локальной сети. Своё прошлое правило с тем же именем заменяется, чужие правила не трогаем.
// Нужны права администратора — будет запрос UAC.
func AllowFirewall(exe string, port int) error {
	if strings.ContainsAny(exe, "\"\r\n%") || port < 1 || port > 65535 {
		return errors.New("странный путь к программе или порт")
	}
	cmd := "cmd.exe"
	if root := os.Getenv("SystemRoot"); root != "" {
		cmd = filepath.Join(root, "System32", "cmd.exe")
	}
	args := fmt.Sprintf(`/c netsh advfirewall firewall delete rule name="%[3]s" & `+
		`netsh advfirewall firewall add rule name="%[3]s" dir=in action=allow program="%[1]s" `+
		`enable=yes profile=private protocol=TCP localport=%[2]d remoteip=localsubnet`, exe, port, FirewallRuleName)
	return runElevatedWait(cmd, args, 90*time.Second)
}

// OpenFirewallApps открывает «Разрешённые приложения» брандмауэра (пользователь сам проверит галочки).
func OpenFirewallApps() error {
	return exec.Command("control.exe", "/name", "Microsoft.WindowsFirewall", "/page", "pageConfigureApps").Start()
}

// OpenNetworkSettings открывает параметры сети (там сеть Wi-Fi переключают на «Частная»).
func OpenNetworkSettings() error { return OpenURL("ms-settings:network-status") }
