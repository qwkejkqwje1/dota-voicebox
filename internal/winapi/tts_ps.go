package winapi

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// Синтез речи через PowerShell: голоса OneCore (WinRT, как в «Параметрах» Windows), иначе System.Speech (SAPI).
// Фразы синтезирует постоянный процесс PowerShell (psWorker): запуск PowerShell и WinRT занимает 1–2 с,
// а фраза «Читалки» должна прозвучать сразу. Если он не запустился — запасной способ: процесс на пачку фраз.

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

// psSynth — синтез одной фразы: у каждой свой голос (id из Voices или подбор по языку и полу; prefer —
// голос, который подходит, если совпадает пол), скорость и файл.
const psSynth = `
$script:rt = $null
$script:rtVoices = @()
$script:rtCur = ''
$script:sapi = $null
function Init-RT {
  if ($null -eq $script:rt) {
    try {
      Init-WinRT
      $script:rtVoices = @([Windows.Media.SpeechSynthesis.SpeechSynthesizer]::AllVoices)
      if ($script:rtVoices.Count -eq 0) { throw 'нет голосов OneCore' }
      $script:rt = New-Object Windows.Media.SpeechSynthesis.SpeechSynthesizer
    } catch {
      [Console]::Error.WriteLine('WinRT: ' + $_.Exception.Message)
      $script:rt = $false
    }
  }
  return ($script:rt -ne $false)
}
function Culture([string]$lang) {
  if ($lang -eq '' -or $lang -eq 'ru') { return 'ru-RU' }
  if ($lang -eq 'en') { return 'en-US' }
  return $lang
}
function Pick-RT($it, [string]$lang) {
  $want = [string]$it.voice
  if ($want.StartsWith('winrt:')) {
    $n = $want.Substring(6)
    $v = $script:rtVoices | Where-Object { $_.DisplayName -eq $n } | Select-Object -First 1
    if ($v) { return $v }
  }
  if ($lang -eq '') { $lang = 'ru' }
  $pool = @($script:rtVoices | Where-Object { $_.Language -like ($lang + '*') })
  if ($pool.Count -eq 0) { return $null }
  $g = [string]$it.gender
  $pref = [string]$it.prefer
  if ($pref.StartsWith('winrt:')) {
    $n = $pref.Substring(6)
    $p = $pool | Where-Object { $_.DisplayName -eq $n -and ($g -eq '' -or [string]$_.Gender -eq $g) } | Select-Object -First 1
    if ($p) { return $p }
  }
  if ($g -ne '') {
    $m = $pool | Where-Object { [string]$_.Gender -eq $g } | Select-Object -First 1
    if ($m) { return $m }
  }
  return $pool[0]
}
function Sapi {
  if ($null -eq $script:sapi) {
    Add-Type -AssemblyName System.Speech
    $script:sapi = New-Object System.Speech.Synthesis.SpeechSynthesizer
  }
  return $script:sapi
}
function Sapi-Has([string]$lang) {
  try {
    $c = Culture $lang
    foreach ($v in (Sapi).GetInstalledVoices()) {
      if ($v.Enabled -and $v.VoiceInfo.Culture.Name -like ($c.Substring(0, 2) + '*')) { return $true }
    }
  } catch {}
  return $false
}
function Save-Part([string]$tmp, [string]$out) {
  [IO.File]::Delete($out)
  [IO.File]::Move($tmp, $out)
}
function Say-RT($it, $v) {
  $s = $script:rt
  if ($script:rtCur -ne $v.Id) { $s.Voice = $v; $script:rtCur = $v.Id }
  try { $s.Options.SpeakingRate = [double]$it.winrt_rate } catch {}
  $st = Await ($s.SynthesizeTextToStreamAsync([string]$it.text)) ([Windows.Media.SpeechSynthesis.SpeechSynthesisStream])
  $size = [uint32]$st.Size
  $r = New-Object Windows.Storage.Streams.DataReader($st.GetInputStreamAt(0))
  Await ($r.LoadAsync($size)) ([uint32]) | Out-Null
  $bytes = New-Object byte[] $size
  $r.ReadBytes($bytes)
  $r.Dispose(); $st.Dispose()
  $tmp = [string]$it.out + '.part'
  [IO.File]::WriteAllBytes($tmp, $bytes)
  Save-Part $tmp ([string]$it.out)
}
function Say-Sapi($it) {
  $s = Sapi
  $want = [string]$it.voice
  $picked = $false
  if ($want.StartsWith('sapi:')) { try { $s.SelectVoice($want.Substring(5)); $picked = $true } catch {} }
  $pref = [string]$it.prefer
  if (-not $picked -and $pref.StartsWith('sapi:')) {
    try {
      $s.SelectVoice($pref.Substring(5))
      $picked = ([string]$it.gender -eq '' -or [string]$s.Voice.Gender -eq [string]$it.gender)
    } catch {}
  }
  if (-not $picked) {
    $g = [System.Speech.Synthesis.VoiceGender]::NotSet
    if ([string]$it.gender -eq 'Male') { $g = [System.Speech.Synthesis.VoiceGender]::Male }
    if ([string]$it.gender -eq 'Female') { $g = [System.Speech.Synthesis.VoiceGender]::Female }
    try { $s.SelectVoiceByHints($g, [System.Speech.Synthesis.VoiceAge]::NotSet, 0, [Globalization.CultureInfo](Culture ([string]$it.lang))) } catch {}
  }
  $s.Rate = [int]$it.rate
  $tmp = [string]$it.out + '.part'
  $s.SetOutputToWaveFile($tmp)
  try { $s.Speak([string]$it.text) } finally { $s.SetOutputToNull() }
  Save-Part $tmp ([string]$it.out)
}
function Synth-One($it) {
  $want = [string]$it.voice
  $lang = [string]$it.lang
  if (-not $want.StartsWith('sapi:') -and (Init-RT)) {
    $v = Pick-RT $it $lang
    # голоса нужного языка нет в OneCore: может, он есть в SAPI; иначе — любой голос OneCore
    if (-not $v -and -not (Sapi-Has $lang)) {
      $v = Pick-RT $it 'ru'
      if (-not $v) { $v = $script:rtVoices[0] }
    }
    if ($v) { Say-RT $it $v; return }
  }
  Say-Sapi $it
}
`

// psRender — запасной способ: один процесс на пачку фраз из файла задания.
const psRender = psPrelude + psSynth + `
$job = Get-Content -Raw -Encoding UTF8 -LiteralPath $env:VB_TTS_JOB | ConvertFrom-Json
foreach ($it in $job.items) {
  try { Synth-One $it } catch { [Console]::Error.WriteLine('TTS: ' + $_.Exception.Message) }
}
`

// psLoop — постоянный процесс: строка на входе — base64(JSON фразы), на выходе ASCII: ready, ok <id>, err <id> <base64>.
const psLoop = `
[Console]::Out.WriteLine('ready')
[Console]::Out.Flush()
while ($true) {
  $line = [Console]::In.ReadLine()
  if ($null -eq $line -or $line -eq 'quit') { break }
  $line = $line.Trim()
  if ($line -eq '') { continue }
  $id = '?'
  try {
    $it = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($line)) | ConvertFrom-Json
    $id = [string]$it.id
    Synth-One $it
    [Console]::Out.WriteLine('ok ' + $id)
  } catch {
    $m = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes([string]$_.Exception.Message))
    [Console]::Out.WriteLine('err ' + $id + ' ' + $m)
  }
  [Console]::Out.Flush()
}
`

const psWorkerScript = psPrelude + psSynth + psLoop

func encodePS(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, v := range u {
		b[2*i], b[2*i+1] = byte(v), byte(v>>8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func psCommand(exe, script string) *exec.Cmd {
	cmd := exec.Command(exe, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePS(script))
	hideWindow(cmd)
	return cmd
}

// tailBuf — последние байты вывода (stderr PowerShell для сообщения об ошибке).
type tailBuf struct {
	mu sync.Mutex
	b  []byte
}

func (t *tailBuf) Write(p []byte) (int, error) {
	t.mu.Lock()
	t.b = append(t.b, p...)
	if len(t.b) > 2048 {
		t.b = append([]byte{}, t.b[len(t.b)-2048:]...)
	}
	t.mu.Unlock()
	return len(p), nil
}

func (t *tailBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.b))
}

// errWorker — сломался сам процесс синтеза (а не одна фраза): фразы синтезируются запасным способом.
var errWorker = errors.New("фоновый синтез речи")

// errWorkerOff — процесс отключён после повторных ошибок (об этом уже написано в журнале).
var errWorkerOff = fmt.Errorf("%w: отключён после ошибок", errWorker)

// psWorker — постоянный процесс PowerShell; закрывается после 10 минут без работы.
type psWorker struct {
	Exe    string // powershell
	Script string
	Idle   time.Duration
	Ready  time.Duration // сколько ждать запуска

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan string
	stderr *tailBuf
	seq    int
	fails  int
	broken bool
	idle   *time.Timer
}

var ttsWorker = &psWorker{Exe: "powershell", Script: psWorkerScript, Idle: 10 * time.Minute, Ready: 25 * time.Second}

func (w *psWorker) startLocked() error {
	cmd := psCommand(w.Exe, w.Script)
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	w.stderr = &tailBuf{}
	cmd.Stderr = w.stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	lines := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			lines <- strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		}
		close(lines)
		cmd.Wait()
	}()
	t := time.NewTimer(w.Ready)
	defer t.Stop()
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				return fmt.Errorf("PowerShell завершился при запуске %s", w.stderr)
			}
			if l == "ready" {
				w.cmd, w.stdin, w.lines = cmd, in, lines
				return nil
			}
		case <-t.C:
			cmd.Process.Kill()
			return fmt.Errorf("PowerShell не запустился за %v", w.Ready)
		}
	}
}

// killLocked останавливает процесс (он сломался или завис).
func (w *psWorker) killLocked() {
	if w.cmd != nil {
		w.stdin.Close()
		w.cmd.Process.Kill()
	}
	w.cmd, w.stdin, w.lines = nil, nil, nil
}

func (w *psWorker) failLocked(err error) error {
	w.killLocked()
	w.fails++
	if w.fails >= 2 && !w.broken {
		w.broken = true
		log.Printf("Синтез речи: фоновый PowerShell не работает (%v) — дальше по-старому, процесс на каждую пачку фраз", err)
	}
	return fmt.Errorf("%w: %v", errWorker, err)
}

// Do синтезирует одну фразу. Ошибка с errWorker — процесс недоступен (стоит попробовать запасной способ).
func (w *psWorker) Do(it ttsItem, timeout time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.broken {
		return errWorkerOff
	}
	if w.cmd == nil {
		if err := w.startLocked(); err != nil {
			return w.failLocked(err)
		}
	}
	w.seq++
	it.ID = strconv.Itoa(w.seq)
	b, _ := json.Marshal(it)
	if _, err := io.WriteString(w.stdin, base64.StdEncoding.EncodeToString(b)+"\n"); err != nil {
		return w.failLocked(err)
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case l, ok := <-w.lines:
			if !ok {
				return w.failLocked(fmt.Errorf("PowerShell завершился %s", w.stderr))
			}
			f := strings.Fields(l)
			if len(f) < 2 || f[1] != it.ID {
				continue
			}
			w.fails = 0
			w.touchLocked()
			switch f[0] {
			case "ok":
				return nil
			case "err":
				msg := "ошибка синтеза"
				if len(f) > 2 {
					if m, err := base64.StdEncoding.DecodeString(f[2]); err == nil {
						msg = strings.TrimSpace(string(m))
					}
				}
				return errors.New(msg)
			}
		case <-t.C:
			return w.failLocked(fmt.Errorf("нет ответа за %v", timeout))
		}
	}
}

// touchLocked — отложить закрытие простаивающего процесса.
func (w *psWorker) touchLocked() {
	if w.Idle <= 0 {
		return
	}
	if w.idle != nil {
		w.idle.Stop()
	}
	w.idle = time.AfterFunc(w.Idle, func() {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.cmd != nil {
			io.WriteString(w.stdin, "quit\n")
			w.stdin.Close()
			w.cmd, w.stdin, w.lines = nil, nil, nil
		}
	})
}

// Close останавливает процесс (выход из программы).
func (w *psWorker) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.idle != nil {
		w.idle.Stop()
	}
	if w.cmd != nil {
		io.WriteString(w.stdin, "quit\n")
		w.stdin.Close()
		w.cmd, w.stdin, w.lines = nil, nil, nil
	}
}
