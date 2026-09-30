//go:build windows

package winapi

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
)

const ttsScript = `
Add-Type -AssemblyName System.Speech
$s = New-Object System.Speech.Synthesis.SpeechSynthesizer
try { $s.SelectVoiceByHints([System.Speech.Synthesis.VoiceGender]::NotSet, [System.Speech.Synthesis.VoiceAge]::NotSet, 0, [Globalization.CultureInfo]'ru-RU') } catch {}
$s.Rate = [int]$env:VB_TTS_RATE
$s.SetOutputToWaveFile($env:VB_TTS_OUT)
$s.Speak($env:VB_TTS_TEXT)
$s.Dispose()
`

// NewTTS возвращает синтезатор речи Windows (System.Speech) с кэшем WAV-файлов.
func NewTTS(cacheDir string, rate int) sounds.TTSFunc {
	return func(text string) (sounds.Clip, error) {
		h := sha1.Sum([]byte(fmt.Sprintf("%d|%s", rate, text)))
		out := filepath.Join(cacheDir, "tts_"+hex.EncodeToString(h[:8])+".wav")
		if _, err := os.Stat(out); err != nil {
			os.MkdirAll(cacheDir, 0o755)
			cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ttsScript)
			cmd.Env = append(os.Environ(), "VB_TTS_TEXT="+text, "VB_TTS_OUT="+out, fmt.Sprintf("VB_TTS_RATE=%d", rate))
			cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
			if b, err := cmd.CombinedOutput(); err != nil {
				return nil, fmt.Errorf("TTS: %v %s", err, b)
			}
		}
		return sounds.DecodeFile(out)
	}
}
