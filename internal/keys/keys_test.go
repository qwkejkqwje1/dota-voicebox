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
