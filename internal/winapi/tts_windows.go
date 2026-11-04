//go:build windows

package winapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// NewTTS — синтез речи Windows: голоса OneCore (WinRT, как в «Параметрах»), иначе System.Speech.
func NewTTS(cacheDir, voice string, rate int) *TTS { return newTTS(cacheDir, voice, rate) }

func hideWindow(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }

const psVoices = psPrelude + `
$list = @()
try {
  Init-WinRT
  foreach ($v in [Windows.Media.SpeechSynthesis.SpeechSynthesizer]::AllVoices) {
    $list += [pscustomobject]@{ id = 'winrt:' + $v.DisplayName; name = $v.DisplayName; lang = $v.Language; gender = [string]$v.Gender }
  }
} catch {}
try {
  Add-Type -AssemblyName System.Speech
  $s = New-Object System.Speech.Synthesis.SpeechSynthesizer
  foreach ($v in $s.GetInstalledVoices()) {
    if (-not $v.Enabled) { continue }
    $i = $v.VoiceInfo
    $list += [pscustomobject]@{ id = 'sapi:' + $i.Name; name = $i.Name + ' (SAPI)'; lang = $i.Culture.Name; gender = [string]$i.Gender }
  }
  $s.Dispose()
} catch {}
ConvertTo-Json -Compress -InputObject @($list)
`

func runPS(script string, env []string, timeout time.Duration) ([]byte, []byte, error) {
	cmd := psCommand("powershell", script)
	cmd.Env = append(os.Environ(), env...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return []byte(out.String()), []byte(errb.String()), err
	case <-time.After(timeout):
		cmd.Process.Kill()
		return nil, nil, fmt.Errorf("PowerShell не ответил за %v", timeout)
	}
}

// synth синтезирует фразы постоянным процессом PowerShell; если он недоступен — одним процессом на пачку.
func (t *TTS) synth(items []ttsItem) error {
	var firstErr error
	for i, it := range items {
		err := ttsWorker.Do(it, 20*time.Second)
		if err == nil {
			continue
		}
		if errors.Is(err, errWorker) {
			if err != errWorkerOff {
				log.Printf("Синтез речи: %v — пробую запасной способ", err)
			}
			if e := synthOnce(t.Dir, items[i:]); e != nil && firstErr == nil {
				firstErr = e
			}
			return firstErr
		}
		if firstErr == nil {
			firstErr = fmt.Errorf("синтез речи: %v", err)
		}
	}
	return firstErr
}

func synthOnce(dir string, items []ttsItem) error {
	b, _ := json.Marshal(map[string]any{"items": items})
	jf := filepath.Join(dir, fmt.Sprintf("tts_job_%d.json", time.Now().UnixNano()))
	if err := os.WriteFile(jf, b, 0o644); err != nil {
		return err
	}
	defer os.Remove(jf)
	_, stderr, err := runPS(psRender, []string{"VB_TTS_JOB=" + jf}, time.Duration(20+5*len(items))*time.Second)
	if err != nil {
		msg := strings.TrimSpace(string(stderr))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return fmt.Errorf("синтез речи: %v %s", err, msg)
	}
	return nil
}

// CloseTTS останавливает фоновый процесс синтеза (при выходе из программы).
func CloseTTS() { ttsWorker.Close() }

var (
	voicesMu    sync.Mutex
	voicesCache []Voice
)

// Voices — установленные голоса (OneCore и SAPI). Список кэшируется.
func (t *TTS) Voices() ([]Voice, error) {
	voicesMu.Lock()
	defer voicesMu.Unlock()
	if voicesCache != nil {
		return voicesCache, nil
	}
	out, stderr, err := runPS(psVoices, nil, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("список голосов: %v %s", err, strings.TrimSpace(string(stderr)))
	}
	var vs []Voice
	s := strings.TrimSpace(strings.TrimPrefix(string(out), "\ufeff"))
	if s != "" && s != "null" {
		if err := json.Unmarshal([]byte(s), &vs); err != nil {
			return nil, fmt.Errorf("список голосов: %v", err)
		}
	}
	voicesCache = vs
	return vs, nil
}
