package script

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gameclock"
)

//go:embed defaults/*.js
var defaults embed.FS

// Info — скрипт для интерфейса.
type Info struct {
	Name    string   `json:"name"`
	Title   string   `json:"title"`
	Enabled bool     `json:"enabled"`
	Running bool     `json:"running"`
	Error   string   `json:"error,omitempty"`
	ErrLine int      `json:"err_line,omitempty"`
	Hooks   []Hook   `json:"hooks"`
	Output  []string `json:"output"`
}

type Manager struct {
	Dir  string
	host Host

	mu      sync.Mutex
	running map[string]*Script
	failed  map[string]*Script
	enabled map[string]bool
	paused  atomic.Bool
}

var nameRe = regexp.MustCompile(`^[\p{L}\p{N}_\-]{1,40}$`)

// ValidName — допустимое имя файла скрипта (без .js).
func ValidName(n string) bool { return nameRe.MatchString(n) }

func NewManager(dir string, host Host) *Manager {
	m := &Manager{Dir: dir, host: host, running: map[string]*Script{}, failed: map[string]*Script{}}
	go m.tickLoop()
	return m
}

// InstallDefaults кладёт примеры скриптов, если папки ещё нет.
func (m *Manager) InstallDefaults() {
	if _, err := os.Stat(m.Dir); err == nil {
		return
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		log.Printf("скрипты: %v", err)
		return
	}
	entries, _ := defaults.ReadDir("defaults")
	for _, e := range entries {
		b, _ := defaults.ReadFile("defaults/" + e.Name())
		os.WriteFile(filepath.Join(m.Dir, e.Name()), b, 0o644)
	}
}

func (m *Manager) path(name string) string { return filepath.Join(m.Dir, name+".js") }

// Names — все скрипты в папке.
func (m *Manager) Names() []string {
	entries, _ := os.ReadDir(m.Dir)
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".js") {
			n := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			if ValidName(n) {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Sync запускает включённые скрипты (перезапуская изменённые) и останавливает выключенные.
func (m *Manager) Sync(enabled map[string]bool) {
	m.mu.Lock()
	m.enabled = enabled
	m.mu.Unlock()
	names := m.Names()
	present := map[string]bool{}
	for _, n := range names {
		present[n] = true
		b, err := os.ReadFile(m.path(n))
		if err != nil {
			continue
		}
		code := string(b)
		m.mu.Lock()
		cur := m.running[n]
		f := m.failed[n]
		m.mu.Unlock()
		switch {
		case !enabled[n]:
			m.stop(n)
		case cur != nil && cur.Code == code:
		case f != nil && f.Code == code:
		default:
			m.restart(n, code)
		}
	}
	m.mu.Lock()
	var gone []string
	for n := range m.running {
		if !present[n] {
			gone = append(gone, n)
		}
	}
	m.mu.Unlock()
	for _, n := range gone {
		m.stop(n)
	}
}

func (m *Manager) stop(name string) {
	m.mu.Lock()
	s := m.running[name]
	delete(m.running, name)
	delete(m.failed, name)
	m.mu.Unlock()
	if s != nil {
		s.close()
		log.Printf("Скрипт %s остановлен", name)
	}
}

func (m *Manager) restart(name, code string) error {
	m.stop(name)
	s := newScript(name, code, m.host)
	err := s.start()
	m.mu.Lock()
	if err != nil {
		m.failed[name] = s
	} else {
		m.running[name] = s
	}
	m.mu.Unlock()
	if err != nil {
		s.close()
		return err
	}
	log.Printf("Скрипт %s запущен", name)
	return nil
}

// Run перезапускает скрипт (даже выключенный — для проверки из редактора).
func (m *Manager) Run(name string) error {
	b, err := os.ReadFile(m.path(name))
	if err != nil {
		return err
	}
	return m.restart(name, string(b))
}

// Read возвращает код скрипта.
func (m *Manager) Read(name string) (string, error) {
	if !ValidName(name) {
		return "", fmt.Errorf("плохое имя скрипта")
	}
	b, err := os.ReadFile(m.path(name))
	return string(b), err
}

// Save сохраняет код. Возвращает ошибку компиляции (файл всё равно сохраняется).
func (m *Manager) Save(name, code string) (compileErr error, err error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("имя скрипта: только буквы, цифры, _ и - (до 40 символов)")
	}
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return nil, err
	}
	tmp := m.path(name) + ".tmp"
	if err := os.WriteFile(tmp, []byte(code), 0o644); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, m.path(name)); err != nil {
		return nil, err
	}
	return Check(name, code), nil
}

func (m *Manager) Delete(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("плохое имя скрипта")
	}
	m.stop(name)
	return os.Remove(m.path(name))
}

var titleRe = regexp.MustCompile(`^\s*//\s*(.+)`)

func (m *Manager) List() []Info {
	m.mu.Lock()
	en := m.enabled
	m.mu.Unlock()
	var out []Info
	for _, n := range m.Names() {
		info := Info{Name: n, Enabled: en[n], Hooks: []Hook{}, Output: []string{}}
		if b, err := os.ReadFile(m.path(n)); err == nil {
			first, _, _ := strings.Cut(string(b), "\n")
			if mt := titleRe.FindStringSubmatch(first); mt != nil {
				info.Title = strings.TrimSpace(mt[1])
			}
		}
		m.mu.Lock()
		s := m.running[n]
		info.Running = s != nil
		if s == nil {
			s = m.failed[n]
		}
		m.mu.Unlock()
		if s != nil {
			s.mu.Lock()
			info.Error, info.ErrLine = s.err, s.errLine
			info.Hooks = append(info.Hooks, s.hooks...)
			info.Output = append(info.Output, s.output...)
			s.mu.Unlock()
		}
		out = append(out, info)
	}
	return out
}

func (m *Manager) each(f func(*Script)) {
	m.mu.Lock()
	list := make([]*Script, 0, len(m.running))
	for _, s := range m.running {
		list = append(list, s)
	}
	m.mu.Unlock()
	for _, s := range list {
		f(s)
	}
}

// SetPaused — временно не реагировать на клавиши (пока назначают клавишу в интерфейсе).
func (m *Manager) SetPaused(on bool) {
	m.paused.Store(on)
	m.each(func(s *Script) { s.matcher.Reset() })
}

// Key — событие клавиатуры/мыши от глобального перехвата.
func (m *Manager) Key(ev combo.Event) {
	if m.paused.Load() {
		return
	}
	m.each(func(s *Script) { s.onKey(ev) })
}

// Clock — тик игрового времени (вызывать несколько раз в секунду).
func (m *Manager) Clock(snap gameclock.Snapshot) { m.each(func(s *Script) { s.onClock(snap) }) }

// Event — игровое событие (kill, death, night, rosh_killed, new_game, state…).
func (m *Manager) Event(name string, data interface{}) {
	m.each(func(s *Script) { s.onEvent(name, data) })
}

// Combos — все клавиши, которые слушают скрипты (для подсказок в интерфейсе).
func (m *Manager) Combos() []string {
	var out []string
	m.each(func(s *Script) {
		s.mu.Lock()
		for _, h := range s.hooks {
			if h.Kind == "hotkey" || h.Kind == "combo" || h.Kind == "hold" || h.Kind == "onKey" {
				out = append(out, h.What)
			}
		}
		s.mu.Unlock()
	})
	return out
}

func (m *Manager) tickLoop() {
	t := time.NewTicker(25 * time.Millisecond)
	defer t.Stop()
	for now := range t.C {
		if !m.paused.Load() {
			m.each(func(s *Script) { s.tick(now) })
		}
	}
}

// Close останавливает все скрипты.
func (m *Manager) Close() {
	m.mu.Lock()
	names := make([]string, 0, len(m.running))
	for n := range m.running {
		names = append(names, n)
	}
	m.mu.Unlock()
	for _, n := range names {
		m.stop(n)
	}
}
