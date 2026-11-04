package winapi

import (
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Скрипты PowerShell проверяются, если есть pwsh (PowerShell 7 для Linux/macOS): VB_PWSH=/путь/к/pwsh.
func pwsh(t *testing.T) string {
	p := os.Getenv("VB_PWSH")
	if p == "" {
		p, _ = exec.LookPath("pwsh")
	}
	if p == "" {
		t.Skip("нет pwsh")
	}
	return p
}

func TestPSScriptsParse(t *testing.T) {
	exe := pwsh(t)
	for name, s := range map[string]string{"render": psRender, "worker": psWorkerScript} {
		chk := `$e = $null; [System.Management.Automation.Language.Parser]::ParseInput([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:VB_S)), [ref]$null, [ref]$e) | Out-Null; foreach ($x in $e) { Write-Output ($x.Extent.StartLineNumber.ToString() + ': ' + $x.Message) }`
		cmd := exec.Command(exe, "-NoProfile", "-NonInteractive", "-Command", chk)
		cmd.Env = append(os.Environ(), "VB_S="+b64(s))
		out, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "" {
			t.Errorf("%s: %v %s", name, err, out)
		}
	}
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

const psFake = `
function Synth-One($it) {
  if ([string]$it.text -eq 'boom') { throw 'не вышло' }
  if ([string]$it.text -eq 'sleep') { Start-Sleep -Seconds 30 }
  [IO.File]::WriteAllText([string]$it.out, [string]$it.text + '|' + [string]$it.gender + '|' + [string]$it.lang)
}
`

func TestPSWorker(t *testing.T) {
	exe := pwsh(t)
	dir := t.TempDir()
	w := &psWorker{Exe: exe, Script: psPrelude + psFake + psLoop, Ready: 30 * time.Second, Idle: time.Hour}
	defer w.Close()
	out := filepath.Join(dir, "a.wav")
	if err := w.Do(ttsItem{Text: "Привет, «мир»", Out: out, Gender: "Male", Lang: "ru"}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "Привет, «мир»|Male|ru" {
		t.Fatalf("файл: %q", b)
	}
	err := w.Do(ttsItem{Text: "boom", Out: filepath.Join(dir, "b.wav")}, 20*time.Second)
	if err == nil || errors.Is(err, errWorker) || !strings.Contains(err.Error(), "не вышло") {
		t.Fatalf("ошибка фразы: %v", err)
	}
	// завис — процесс убит, следующая фраза запускает новый
	err = w.Do(ttsItem{Text: "sleep", Out: filepath.Join(dir, "c.wav")}, 2*time.Second)
	if !errors.Is(err, errWorker) {
		t.Fatalf("зависание: %v", err)
	}
	out2 := filepath.Join(dir, "d.wav")
	if err := w.Do(ttsItem{Text: "ещё", Out: out2}, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	if w.broken {
		t.Fatal("одна ошибка не должна отключать процесс")
	}
}

// Настоящий скрипт вне Windows: голосов нет — ошибка фразы, но протокол работает.
func TestPSWorkerRealScript(t *testing.T) {
	exe := pwsh(t)
	w := &psWorker{Exe: exe, Script: psWorkerScript, Ready: 30 * time.Second}
	defer w.Close()
	err := w.Do(ttsItem{Text: "проверка", Out: filepath.Join(t.TempDir(), "x.wav"), Lang: "ru"}, 20*time.Second)
	if err == nil || errors.Is(err, errWorker) {
		t.Fatalf("ждали ошибку синтеза, а не процесса: %v", err)
	}
	t.Log(err)
}
