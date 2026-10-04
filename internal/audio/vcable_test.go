package audio

import "testing"

func devs(n ...string) []Device {
	var d []Device
	for _, x := range n {
		d = append(d, Device{Name: x})
	}
	return d
}

func TestFindCable(t *testing.T) {
	cases := []struct {
		play []string
		want string
	}{
		{[]string{"Speakers (Realtek(R) Audio)", "CABLE Input (VB-Audio Virtual Cable)"}, "CABLE Input (VB-Audio Virtual Cable)"},
		{[]string{"Динамики (Realtek)", "Line 1 (Virtual Audio Cable)"}, "Line 1 (Virtual Audio Cable)"},
		{[]string{"Line 2 (Virtual Audio Cable)", "Line 1 (Virtual Audio Cable)"}, "Line 1 (Virtual Audio Cable)"},
		{[]string{"Line 1"}, "Line 1"},
		{[]string{"Speakers", "Voicemeeter Input (VB-Audio Voicemeeter VAIO)"}, "Voicemeeter Input (VB-Audio Voicemeeter VAIO)"},
		{[]string{"Speakers (Realtek)", "Headphones (USB)"}, ""},
		{[]string{"Line In (Realtek)"}, ""},
	}
	for _, c := range cases {
		if g := FindCable(devs(c.play...)); g != c.want {
			t.Errorf("%v: got %q want %q", c.play, g, c.want)
		}
	}
}

func TestCablePair(t *testing.T) {
	cap := devs("Microphone (Realtek)", "CABLE Output (VB-Audio Virtual Cable)", "Line 1 (Virtual Audio Cable)", "Line 10 (Virtual Audio Cable)",
		"Voicemeeter Out B1 (VB-Audio Voicemeeter VAIO)", "Voicemeeter Out B2 (VB-Audio Voicemeeter AUX VAIO)", "CABLE-A Output (VB-Audio Cable A)")
	cases := map[string]string{
		"CABLE Input (VB-Audio Virtual Cable)":                  "CABLE Output (VB-Audio Virtual Cable)",
		"Line 1 (Virtual Audio Cable)":                          "Line 1 (Virtual Audio Cable)",
		"Line 10 (Virtual Audio Cable)":                         "Line 10 (Virtual Audio Cable)",
		"Line 1":                                                "Line 1 (Virtual Audio Cable)",
		"Voicemeeter Input (VB-Audio Voicemeeter VAIO)":         "Voicemeeter Out B1 (VB-Audio Voicemeeter VAIO)",
		"Voicemeeter AUX Input (VB-Audio Voicemeeter AUX VAIO)": "Voicemeeter Out B2 (VB-Audio Voicemeeter AUX VAIO)",
		"CABLE-A Input (VB-Audio Cable A)":                      "CABLE-A Output (VB-Audio Cable A)",
	}
	for play, want := range cases {
		if g := CablePair(play, cap); g != want {
			t.Errorf("%s: got %q want %q", play, g, want)
		}
	}
}

func TestMatchDevice(t *testing.T) {
	n := []string{"Line 10 (Virtual Audio Cable)", "Line 1 (Virtual Audio Cable)"}
	if i := matchDevice(n, "Line 1"); i != 1 {
		t.Fatal("Line 1 →", i)
	}
	if i := matchDevice(n, "line 1 (virtual audio cable)"); i != 1 {
		t.Fatal(i)
	}
	if i := matchDevice(n, "Virtual"); i != 0 {
		t.Fatal(i)
	}
	if !IsVirtual("Line 1") || IsVirtual("Line In (Realtek)") || IsVirtual("Microphone (Realtek)") {
		t.Fatal("IsVirtual")
	}
}
