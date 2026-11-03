//go:build windows

package winapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
)

// NewTTS — синтез речи Windows: голоса OneCore (WinRT, как в «Параметрах»), иначе System.Speech.
func NewTTS(cacheDir, voice string, rate int) *TTS { return newTTS(cacheDir, voice, rate) }

// Общая часть скриптов: UTF-8 вывод и функция ожидания асинхронных операций WinRT.
const psPrelude = `
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [Text.Encoding]::UTF8
function Init-WinRT {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  [Windows.Media.SpeechSynthesis.SpeechSynthesizer, Windows.Media.SpeechSynthesis, ContentType = WindowsRuntime] | Out-Null
  [Windows.Storage.Streams.DataReader, Windows.Storage.Streams, ContentType = WindowsRuntime] | Out-Null
  $script:asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
}
function Await($op, [Type]$t) {
  $task = $script:asTask.MakeGenericMethod($t).Invoke($null, @($op))
  $task.Wait() | Out-Null
  $task.Result
}
`

const psRender = psPrelude + `
$job = Get-Content -Raw -Encoding UTF8 -LiteralPath $env:VB_TTS_JOB | ConvertFrom-Json
$want = [string]$job.voice
$ok = $false
if (-not $want.StartsWith('sapi:')) {
  try {
    Init-WinRT
    $s = New-Object Windows.Media.SpeechSynthesis.SpeechSynthesizer
    $all = [Windows.Media.SpeechSynthesis.SpeechSynthesizer]::AllVoices
    $v = $null
    if ($want.StartsWith('winrt:')) { $n = $want.Substring(6); $v = $all | Where-Object { $_.DisplayName -eq $n } | Select-Object -First 1 }
    if (-not $v) { $v = $all | Where-Object { $_.Language -like 'ru*' } | Select-Object -First 1 }
    if (-not $v -and $want -eq '') { throw 'нет русского голоса OneCore' }
    if ($v) { $s.Voice = $v }
    try { $s.Options.SpeakingRate = [double]$job.winrt_rate } catch {}
    foreach ($it in $job.items) {
      $st = Await ($s.SynthesizeTextToStreamAsync([string]$it.text)) ([Windows.Media.SpeechSynthesis.SpeechSynthesisStream])
      $size = [uint32]$st.Size
      $r = New-Object Windows.Storage.Streams.DataReader($st.GetInputStreamAt(0))
      Await ($r.LoadAsync($size)) ([uint32]) | Out-Null
      $bytes = New-Object byte[] $size
      $r.ReadBytes($bytes)
      [IO.File]::WriteAllBytes([string]$it.out, $bytes)
      $r.Dispose(); $st.Dispose()
    }
    $ok = $true
  } catch { [Console]::Error.WriteLine('WinRT: ' + $_.Exception.Message) }
}
if (-not $ok) {
  Add-Type -AssemblyName System.Speech
  $s = New-Object System.Speech.Synthesis.SpeechSynthesizer
  $picked = $false
  if ($want.StartsWith('sapi:')) { try { $s.SelectVoice($want.Substring(5)); $picked = $true } catch {} }
  if (-not $picked) { try { $s.SelectVoiceByHints([System.Speech.Synthesis.VoiceGender]::NotSet, [System.Speech.Synthesis.VoiceAge]::NotSet, 0, [Globalization.CultureInfo]'ru-RU') } catch {} }
  $s.Rate = [int]$job.rate
  foreach ($it in $job.items) {
    if (Test-Path -LiteralPath ([string]$it.out)) { continue }
    $s.SetOutputToWaveFile([string]$it.out)
    $s.Speak([string]$it.text)
  }
  $s.SetOutputToNull()
  $s.Dispose()
}
`

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

func encodePS(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[2*i], b[2*i+1] = byte(v), byte(v>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func runPS(script string, env []string, timeout time.Duration) ([]byte, []byte, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePS(script))
	cmd.Env = append(os.Environ(), env...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
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

func (t *TTS) synth(items []ttsItem) error {
	job := map[string]any{"voice": t.Voice, "rate": t.Rate, "winrt_rate": RateWinRT(t.Rate), "items": items}
	b, _ := json.Marshal(job)
	jf := filepath.Join(t.Dir, fmt.Sprintf("tts_job_%d.json", time.Now().UnixNano()))
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
