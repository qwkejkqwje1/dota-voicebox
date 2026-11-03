// Package overlay рисует быстрое меню и всплывающие уведомления поверх игры.
// Только чистый Go (картинка RGBA с premultiplied alpha) — окно Windows (layered window)
// просто показывает готовое изображение. Так меню можно проверить тестом и сохранить в PNG.
package overlay

import (
	"image"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// Fonts — шрифты меню: обычный, полужирный и (необязательно) эмодзи.
type Fonts struct {
	Regular *opentype.Font
	Bold    *opentype.Font
	Emoji   *opentype.Font // монохромные контуры эмодзи (Segoe UI Emoji); nil — нет

	mu        sync.Mutex
	faces     map[faceKey]font.Face
	emojiPath []string // шрифт эмодзи большой — читаем только когда понадобится
	emojiOnce sync.Once
}

// emoji — шрифт эмодзи (загружается при первом обращении).
func (f *Fonts) emoji() *opentype.Font {
	f.emojiOnce.Do(func() {
		if f.Emoji == nil && len(f.emojiPath) > 0 {
			f.Emoji = parseFile(f.emojiPath...)
		}
	})
	return f.Emoji
}

type faceKey struct {
	f  *opentype.Font
	px int
}

func parseFile(paths ...string) *opentype.Font {
	for _, p := range paths {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if f, err := opentype.Parse(b); err == nil {
			return f
		}
	}
	return nil
}

// LoadFonts ищет Segoe UI в папке шрифтов Windows; иначе — системные шрифты Linux или встроенный Go-шрифт.
func LoadFonts() *Fonts {
	win := os.Getenv("WINDIR")
	if win == "" {
		win = `C:\Windows`
	}
	wf := func(name string) string { return filepath.Join(win, "Fonts", name) }
	f := &Fonts{faces: map[faceKey]font.Face{}}
	f.Regular = parseFile(os.Getenv("VOICEBOX_FONT"), wf("segoeui.ttf"),
		"/usr/share/fonts/liberation-sans/LiberationSans-Regular.ttf", "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf")
	f.Bold = parseFile(os.Getenv("VOICEBOX_FONT_BOLD"), wf("seguisb.ttf"), wf("segoeuib.ttf"),
		"/usr/share/fonts/liberation-sans/LiberationSans-Bold.ttf", "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf")
	f.emojiPath = []string{os.Getenv("VOICEBOX_EMOJI_FONT"), wf("seguiemj.ttf")}
	if f.Regular == nil {
		f.Regular, _ = opentype.Parse(goregular.TTF)
	}
	if f.Bold == nil {
		f.Bold, _ = opentype.Parse(gobold.TTF)
	}
	return f
}

func (f *Fonts) face(o *opentype.Font, px float64) font.Face {
	if o == nil {
		return nil
	}
	k := faceKey{o, int(px*4 + 0.5)}
	f.mu.Lock()
	defer f.mu.Unlock()
	if fc, ok := f.faces[k]; ok {
		return fc
	}
	fc, err := opentype.NewFace(o, &opentype.FaceOptions{Size: float64(k.px) / 4, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil
	}
	f.faces[k] = fc
	return fc
}

// EmojiSource — цветные картинки эмодзи (их рисует интерфейс в WebView и сохраняет в кэш).
type EmojiSource interface {
	Emoji(e string) image.Image
}
