package overlay

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EmojiKey — имя файла для эмодзи: коды символов через «-» (как у twemoji), без FE0F.
func EmojiKey(e string) string {
	var parts []string
	for _, r := range e {
		if r == 0xFE0F {
			continue
		}
		parts = append(parts, strconv.FormatInt(int64(r), 16))
	}
	return strings.Join(parts, "-")
}

// FileEmoji — цветные эмодзи из папки с PNG. Картинки рисует интерфейс (WebView2 умеет
// цветные эмодзи Windows) и сохраняет сюда, а меню поверх игры берёт готовые.
type FileEmoji struct {
	Dir string

	mu      sync.Mutex
	cache   map[string]image.Image
	missing map[string]time.Time
}

// Emoji — картинка или nil (тогда меню нарисует контурный значок).
func (f *FileEmoji) Emoji(e string) image.Image {
	if f == nil || e == "" {
		return nil
	}
	k := EmojiKey(e)
	f.mu.Lock()
	defer f.mu.Unlock()
	if img, ok := f.cache[k]; ok {
		return img
	}
	if t, ok := f.missing[k]; ok && time.Since(t) < 5*time.Second {
		return nil
	}
	img, err := f.load(k)
	if err != nil {
		if f.missing == nil {
			f.missing = map[string]time.Time{}
		}
		f.missing[k] = time.Now()
		return nil
	}
	if f.cache == nil {
		f.cache = map[string]image.Image{}
	}
	f.cache[k] = img
	delete(f.missing, k)
	return img
}

func (f *FileEmoji) load(k string) (image.Image, error) {
	b, err := os.ReadFile(filepath.Join(f.Dir, k+".png"))
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(b))
}

// Has — есть ли картинка (для интерфейса: какие ещё нарисовать).
func (f *FileEmoji) Has(e string) bool {
	_, err := os.Stat(filepath.Join(f.Dir, EmojiKey(e)+".png"))
	return err == nil
}

// Save сохраняет PNG от интерфейса (проверяет, что это небольшая картинка).
func (f *FileEmoji) Save(e string, data []byte) error {
	k := EmojiKey(e)
	if k == "" || len(k) > 80 || len(data) > 256<<10 {
		return errors.New("неверный эмодзи")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if cfg.Width < 8 || cfg.Width > 256 || cfg.Height < 8 || cfg.Height > 256 {
		return errors.New("неверный размер картинки")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.Dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(f.Dir, k+".png"), data, 0o644); err != nil {
		return err
	}
	f.mu.Lock()
	if f.cache == nil {
		f.cache = map[string]image.Image{}
	}
	f.cache[k] = img
	delete(f.missing, k)
	f.mu.Unlock()
	return nil
}
