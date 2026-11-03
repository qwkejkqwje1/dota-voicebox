package keys

import "testing"

func TestParse(t *testing.T) {
	c, err := Parse("Ctrl+Alt+Num1")
	if err != nil || c.VK != 0x61 || c.Mods != ModCtrl|ModAlt {
		t.Fatalf("got %+v %v", c, err)
	}
	if c, err := Parse("f9"); err != nil || c.VK != 0x78 {
		t.Fatalf("got %+v %v", c, err)
	}
	if _, err := Parse("Hyper+X"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDisplay(t *testing.T) {
	for in, want := range map[string]string{"Alt+TILDE": "Alt+~", "ctrl+num1": "Ctrl+Num 1", "Ctrl+Alt+V": "Ctrl+Alt+V", "Pause": "Pause", "Mouse4": "Мышь 4", "": "", "F9": "F9", "Shift+NumAdd": "Shift+Num +"} {
		if got := Display(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
