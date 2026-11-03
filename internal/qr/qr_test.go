package qr

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapacityTable(t *testing.T) {
	// известные значения ёмкости байтового режима (ISO/IEC 18004, таблица 7)
	cases := []struct {
		ver  int
		lvl  Level
		want int
	}{{1, L, 17}, {1, M, 14}, {1, Q, 11}, {1, H, 7}, {2, M, 26}, {3, M, 42}, {4, M, 62}, {5, Q, 60}, {7, M, 122},
		{10, L, 271}, {10, H, 119}, {15, M, 412}, {20, Q, 482}, {27, L, 1465}, {40, L, 2953}, {40, H, 1273}}
	for _, c := range cases {
		if got := capacity(c.ver, c.lvl); got != c.want {
			t.Errorf("capacity(%d, %d) = %d, want %d", c.ver, c.lvl, got, c.want)
		}
	}
}

func TestEncodeBasics(t *testing.T) {
	c, err := Encode("http://192.168.1.23:3002/abcd2345efgh/", M)
	if err != nil {
		t.Fatal(err)
	}
	if c.Version != 3 || c.Size != 29 {
		t.Fatalf("version %d size %d", c.Version, c.Size)
	}
	// угловые искатели: тёмный край, светлое кольцо, тёмный центр
	for _, p := range [][2]int{{0, 0}, {c.Size - 7, 0}, {0, c.Size - 7}} {
		if !c.Dark(p[0], p[1]) || c.Dark(p[0]+1, p[1]+1) || !c.Dark(p[0]+3, p[1]+3) {
			t.Fatalf("finder at %v broken", p)
		}
	}
	if !c.Dark(8, c.Size-8) {
		t.Fatal("dark module missing")
	}
	svg := c.SVG("", "")
	if !strings.HasPrefix(svg, "<svg") || !strings.Contains(svg, "viewBox=\"0 0 37 37\"") {
		t.Fatal(svg[:80])
	}
	if _, err := Encode(strings.Repeat("x", 3000), L); err == nil {
		t.Fatal("too long accepted")
	}
}

// QR_PNG=dir — сохранить картинки для проверки сторонним распознавателем.
func TestWritePNG(t *testing.T) {
	dir := os.Getenv("QR_PNG")
	if dir == "" {
		t.Skip("QR_PNG не задан")
	}
	var list []string
	for v, text := range []string{
		"http://10.0.0.7:3002/k3/",
		"http://192.168.1.23:3002/abcd2345efgh/",
		"http://192.168.100.123:30002/abcdefghjkmnpqrs/?x=Привет",
		strings.Repeat("VoiceBox-", 12),
		strings.Repeat("0123456789", 22),
		strings.Repeat("пульт ", 40),
	} {
		for _, lvl := range []Level{L, M, Q, H} {
			c, err := Encode(text, lvl)
			if err != nil {
				t.Fatal(err)
			}
			const k, border = 8, 4
			n := (c.Size + 2*border) * k
			img := image.NewGray(image.Rect(0, 0, n, n))
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					v := uint8(255)
					if c.Dark(x/k-border, y/k-border) {
						v = 0
					}
					img.SetGray(x, y, color.Gray{v})
				}
			}
			name := fmt.Sprintf("qr_%d_%d_v%d_m%d.png", v, lvl, c.Version, c.Mask)
			f, _ := os.Create(filepath.Join(dir, name))
			png.Encode(f, img)
			f.Close()
			list = append(list, name+"\t"+text)
		}
	}
	os.WriteFile(filepath.Join(dir, "index.tsv"), []byte(strings.Join(list, "\n")), 0o644)
}
