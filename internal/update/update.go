// Package update — автообновление из GitHub Releases.
//
// Безопасность: каждый релиз подписан ключом ed25519 (секрет CI). Подпись покрывает
// имя версии и SHA-256 файла voicebox.exe, поэтому подменить файл или «откатить»
// на старую версию под видом новой нельзя. Открытый ключ встроен в программу.
//
// Установка: новый exe кладётся рядом, текущий переименовывается в voicebox.exe.old
// (Windows разрешает переименовать запущенный файл), затем запускается «сторож» —
// старая версия с флагом -update-guard. Если новая версия не подтвердит, что
// нормально запустилась, сторож вернёт старую.
package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PublicKey — открытый ключ подписи релизов (base64).
const PublicKey = "c85U9fLFlrjp0l1e7VHqQebj4eVwHc5Zosy3mJmM2vY="

const (
	Repo      = "qwkejkqwje1/dota-voicebox"
	ExeAsset  = "voicebox.exe"
	SigAsset  = "voicebox.exe.sig"
	okMarker  = "update.ok"
	healthyIn = 15 * time.Second
)

type Release struct {
	Tag    string `json:"tag"`
	Notes  string `json:"notes"`
	URL    string `json:"url"`
	exeURL string
	sigURL string
}

type Status struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest,omitempty"`
	Available   bool      `json:"available"`
	State       string    `json:"state"` // idle | checking | uptodate | available | downloading | ready | error
	Error       string    `json:"error,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	URL         string    `json:"url,omitempty"`
	Progress    float64   `json:"progress"`
	CheckedAt   time.Time `json:"checked_at"`
	CanRollback bool      `json:"can_rollback"`
	Previous    string    `json:"previous,omitempty"`
}

type Updater struct {
	Current string
	Exe     string // путь к текущему voicebox.exe
	API     string // https://api.github.com (подменяется в тестах)
	Client  *http.Client
	Pub     ed25519.PublicKey

	mu     sync.Mutex
	st     Status
	rel    *Release
	staged string
}

func New(current, exe string) *Updater {
	pub, _ := base64.StdEncoding.DecodeString(PublicKey)
	u := &Updater{Current: current, Exe: exe, API: "https://api.github.com", Client: &http.Client{Timeout: 60 * time.Second}, Pub: pub}
	u.st = Status{Current: current, State: "idle"}
	if b, err := os.ReadFile(exe + ".old.version"); err == nil {
		if _, err := os.Stat(exe + ".old"); err == nil {
			u.st.CanRollback, u.st.Previous = true, strings.TrimSpace(string(b))
		}
	}
	return u
}

func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

func (u *Updater) set(f func(*Status)) {
	u.mu.Lock()
	f(&u.st)
	u.mu.Unlock()
}

// Newer — версия a новее b ("v0.4.1" > "v0.4.0"). "dev" никогда не новее и всегда старше.
func Newer(a, b string) bool {
	pa, oka := parse(a)
	pb, okb := parse(b)
	if !oka {
		return false
	}
	if !okb {
		return false // сборка разработчика не обновляется сама
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	// v0.5.0 новее, чем v0.5.0-dev.1004 (предварительная сборка)
	return !strings.Contains(a, "-") && strings.Contains(b, "-")
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Message — то, что подписывается: версия + хеш файла.
func Message(tag string, exe []byte) []byte {
	sum := sha256.Sum256(exe)
	return []byte("voicebox-release:" + tag + ":" + hex.EncodeToString(sum[:]))
}

// Verify проверяет подпись (sig — base64).
func Verify(pub ed25519.PublicKey, tag string, exe []byte, sig string) error {
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("в программе нет ключа проверки обновлений")
	}
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sig))
	if err != nil || !ed25519.Verify(pub, Message(tag, exe), s) {
		return errors.New("подпись обновления неверна — файл не установлен")
	}
	if len(exe) < 2 || exe[0] != 'M' || exe[1] != 'Z' {
		return errors.New("файл обновления повреждён")
	}
	return nil
}

// Check спрашивает GitHub о последнем релизе.
func (u *Updater) Check() (*Release, error) {
	u.set(func(s *Status) { s.State, s.Error = "checking", "" })
	rel, err := u.latest()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.st.CheckedAt = time.Now()
	if err != nil {
		u.st.State, u.st.Error = "error", err.Error()
		return nil, err
	}
	u.st.Latest, u.st.Notes, u.st.URL = rel.Tag, rel.Notes, rel.URL
	u.st.Available = Newer(rel.Tag, u.Current) && rel.exeURL != "" && rel.sigURL != ""
	switch {
	case u.staged != "" && u.rel != nil && u.rel.Tag == rel.Tag:
		u.st.State = "ready"
	case u.st.Available:
		u.st.State = "available"
		u.rel = rel
	default:
		u.st.State = "uptodate"
	}
	return rel, nil
}

func (u *Updater) latest() (*Release, error) {
	req, _ := http.NewRequest("GET", u.API+"/repos/"+Repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "VoiceBox/"+u.Current)
	r, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("нет связи с GitHub: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub ответил %s", r.Status)
	}
	var gh struct {
		Tag    string `json:"tag_name"`
		Body   string `json:"body"`
		URL    string `json:"html_url"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&gh); err != nil {
		return nil, err
	}
	rel := &Release{Tag: gh.Tag, Notes: gh.Body, URL: gh.URL}
	for _, a := range gh.Assets {
		switch a.Name {
		case ExeAsset:
			rel.exeURL = a.URL
		case SigAsset:
			rel.sigURL = a.URL
		}
	}
	return rel, nil
}

func (u *Updater) get(url string, limit int64, progress func(float64)) ([]byte, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "VoiceBox/"+u.Current)
	r, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("загрузка: %s", r.Status)
	}
	var buf []byte
	tmp := make([]byte, 64<<10)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if int64(len(buf)) > limit {
			return nil, errors.New("файл слишком большой")
		}
		if progress != nil && r.ContentLength > 0 {
			progress(float64(len(buf)) / float64(r.ContentLength))
		}
		if err == io.EOF {
			return buf, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// Download скачивает и проверяет новую версию; файл кладётся рядом как voicebox.exe.new.
func (u *Updater) Download() error {
	u.mu.Lock()
	rel := u.rel
	u.mu.Unlock()
	if rel == nil {
		return errors.New("сначала проверьте обновления")
	}
	u.set(func(s *Status) { s.State, s.Progress, s.Error = "downloading", 0, "" })
	fail := func(err error) error {
		u.set(func(s *Status) { s.State, s.Error = "error", err.Error() })
		return err
	}
	sig, err := u.get(rel.sigURL, 4096, nil)
	if err != nil {
		return fail(err)
	}
	exe, err := u.get(rel.exeURL, 200<<20, func(p float64) { u.set(func(s *Status) { s.Progress = p }) })
	if err != nil {
		return fail(err)
	}
	if err := Verify(u.Pub, rel.Tag, exe, string(sig)); err != nil {
		return fail(err)
	}
	staged := u.Exe + ".new"
	if err := os.WriteFile(staged, exe, 0o755); err != nil {
		return fail(fmt.Errorf("не удалось сохранить обновление: %w", err))
	}
	u.mu.Lock()
	u.staged = staged
	u.st.State, u.st.Progress = "ready", 1
	u.mu.Unlock()
	log.Printf("Обновление %s скачано и проверено (подпись верна)", rel.Tag)
	return nil
}

// Install меняет файлы и запускает сторожа. После успешного вызова программа должна выйти.
func (u *Updater) Install(args []string) error {
	u.mu.Lock()
	staged, rel := u.staged, u.rel
	u.mu.Unlock()
	if staged == "" || rel == nil {
		return errors.New("обновление ещё не скачано")
	}
	old := u.Exe + ".old"
	os.Remove(old)
	if err := os.Rename(u.Exe, old); err != nil {
		return fmt.Errorf("не удалось заменить файл (нет прав на папку?): %w", err)
	}
	if err := os.Rename(staged, u.Exe); err != nil {
		os.Rename(old, u.Exe)
		return fmt.Errorf("не удалось заменить файл: %w", err)
	}
	os.WriteFile(u.Exe+".old.version", []byte(u.Current), 0o644)
	os.Remove(markerPath(u.Exe))
	guardArgs := append([]string{"-update-guard", u.Exe}, args...)
	cmd := exec.Command(old, guardArgs...)
	if err := cmd.Start(); err != nil {
		// сторож не запустился — откатываем замену, чтобы не остаться без проверки
		os.Rename(u.Exe, staged)
		os.Rename(old, u.Exe)
		return fmt.Errorf("не удалось запустить установку: %w", err)
	}
	log.Printf("Устанавливаю %s, программа перезапустится…", rel.Tag)
	return nil
}

func markerPath(exe string) string { return exe + "." + okMarker }

// MarkHealthy вызывается новой версией после запуска с -post-update:
// через 15 секунд нормальной работы сторож получает подтверждение.
func MarkHealthy(exe string) {
	go func() {
		time.Sleep(healthyIn)
		os.WriteFile(markerPath(exe), []byte("ok"), 0o644)
	}()
}

// Guard — режим сторожа (запускается из старого exe). ping — проверка, что прежний
// процесс освободил порт интерфейса (false = освободил).
func Guard(exe string, args []string, ping func() bool) {
	logf := func(f string, a ...any) {
		lf, err := os.OpenFile(exe+".update.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(lf, time.Now().Format("2006-01-02 15:04:05 ")+f+"\n", a...)
			lf.Close()
		}
	}
	for i := 0; i < 100 && ping(); i++ { // ждём выхода старой копии (до 20 с)
		time.Sleep(200 * time.Millisecond)
	}
	start := func(path string, extra ...string) (*exec.Cmd, error) {
		c := exec.Command(path, append(extra, args...)...)
		return c, c.Start()
	}
	cmd, err := start(exe, "-post-update")
	if err != nil {
		logf("новая версия не запускается: %v — откат", err)
		rollbackFiles(exe)
		start(exe)
		return
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.After(healthyIn + 75*time.Second)
	for {
		select {
		case err := <-exited:
			if _, e := os.Stat(markerPath(exe)); e == nil {
				logf("обновление прошло успешно")
				return
			}
			logf("новая версия завершилась до подтверждения (%v) — откат", err)
			rollbackFiles(exe)
			start(exe)
			return
		case <-time.After(time.Second):
			if _, e := os.Stat(markerPath(exe)); e == nil {
				logf("обновление прошло успешно")
				return
			}
		case <-deadline:
			logf("нет подтверждения от новой версии — оставляю её работать")
			return
		}
	}
}

func rollbackFiles(exe string) error {
	old := exe + ".old"
	if _, err := os.Stat(old); err != nil {
		return errors.New("нет прошлой версии для отката")
	}
	os.Remove(exe + ".bad")
	if err := os.Rename(exe, exe+".bad"); err != nil {
		return err
	}
	if err := os.Rename(old, exe); err != nil {
		os.Rename(exe+".bad", exe)
		return err
	}
	os.Remove(exe + ".old.version")
	return nil
}

// Rollback — ручной откат на прошлую версию (кнопка в интерфейсе). Затем нужен перезапуск.
func (u *Updater) Rollback(args []string) error {
	if err := rollbackFiles(u.Exe); err != nil {
		return err
	}
	// запускаем восстановленную версию через сторожа-ожидание выхода текущей
	cmd := exec.Command(u.Exe, append([]string{"-wait-restart"}, args...)...)
	return cmd.Start()
}
