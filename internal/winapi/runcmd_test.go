package winapi

import "testing"

func TestRunTarget(t *testing.T) {
	for in, want := range map[string]string{
		`"C:\Games\VoiceBox\voicebox.exe" -background`: `C:\Games\VoiceBox\voicebox.exe`,
		`C:\Tools\voicebox.exe -background`:            `C:\Tools\voicebox.exe`,
		`  "D:\Мои программы\vb (1).exe"`:              `D:\Мои программы\vb (1).exe`,
		`C:\x\VoiceBox.EXE`:                            `C:\x\VoiceBox.EXE`,
	} {
		if got := runTarget(in); got != want {
			t.Errorf("runTarget(%q) = %q, want %q", in, got, want)
		}
	}
}
