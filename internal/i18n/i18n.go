// Package i18n — английский интерфейс. Исходные строки программы — русские; перевод — словарь en.json,
// где ключ — русская строка, значение — английская. Строки с глаголами fmt (%s, %d, %q…) — шаблоны:
// «звук %q не найден» переводит и готовую строку «звук "x" не найден» (подставленное переводится тоже).
package i18n

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

//go:embed en.json
var enJSON []byte

var lang atomic.Value

func init() { lang.Store("ru") }

// SetLang — язык интерфейса: "en" — английский, всё остальное — русский.
func SetLang(l string) {
	if l != "en" {
		l = "ru"
	}
	lang.Store(l)
}

// Lang — текущий язык интерфейса ("ru" | "en").
func Lang() string { return lang.Load().(string) }

// En — интерфейс на английском.
func En() bool { return Lang() == "en" }

type pattern struct {
	re     *regexp.Regexp
	key    string // самый длинный кусок текста шаблона — быстрая проверка до регулярки
	out    []piece
	size   int          // длина текста без глаголов: более конкретные шаблоны — первыми
	quoted map[int]bool // группы из %q — названия в кавычках (данные пользователя), их не переводим
}

// piece — кусок перевода: текст или номер подстановки (с 1).
type piece struct {
	text string
	arg  int
}

var (
	loadOnce sync.Once
	dict     map[string]string
	pats     []pattern

	cacheMu sync.Mutex
	cache   = map[string]string{}
)

// verbRe — глаголы fmt: %s, %q, %d, %v, %g, %.1f, %5.2f, %[2]s…; %% — просто знак процента.
var verbRe = regexp.MustCompile(`%%|%(\[(\d+)\])?[-+# 0]*\d*(\.\d+)?[svqdgfeExXcwtTbo]`)

var cyr = regexp.MustCompile(`[А-Яа-яЁё]`)

func load() {
	loadOnce.Do(func() {
		d := map[string]string{}
		if err := json.Unmarshal(enJSON, &d); err != nil {
			log.Printf("i18n: en.json: %v", err)
		}
		use(d)
	})
}

// use — словарь (en.json или тестовый).
func use(d map[string]string) {
	cacheMu.Lock()
	cache = map[string]string{}
	cacheMu.Unlock()
	dict, pats = d, nil
	{
		for k, v := range dict {
			if !verbRe.MatchString(k) {
				continue
			}
			if p, ok := compile(k, v); ok {
				pats = append(pats, p)
			}
		}
		sort.SliceStable(pats, func(i, j int) bool {
			if pats[i].size != pats[j].size {
				return pats[i].size > pats[j].size
			}
			return pats[i].re.String() < pats[j].re.String()
		})
	}
}

// compile — шаблон из пары «русский формат → английский формат».
func compile(ru, en string) (pattern, bool) {
	var re strings.Builder
	re.WriteString(`^(?s)`)
	last, n, size := 0, 0, 0
	longest := ""
	lit := func(s string) {
		re.WriteString(regexp.QuoteMeta(s))
		size += len(s)
		if len(s) > len(longest) {
			longest = s
		}
	}
	group := map[int]int{} // номер аргумента fmt → номер группы в регулярке
	quoted := map[int]bool{}
	next := 1
	for _, m := range verbRe.FindAllStringSubmatchIndex(ru, -1) {
		lit(ru[last:m[0]])
		if ru[m[0]:m[1]] == "%%" {
			lit("%")
		} else {
			re.WriteString(`(.+?)`)
			n++
			arg := next
			if m[4] >= 0 {
				arg, _ = strconv.Atoi(ru[m[4]:m[5]])
			}
			group[arg] = n
			if strings.HasSuffix(ru[m[0]:m[1]], "q") {
				quoted[n] = true
			}
			next = arg + 1
		}
		last = m[1]
	}
	lit(ru[last:])
	re.WriteString(`$`)
	// шаблон без русского текста («%s: %s») подходит к чему угодно — такие не используем
	if n == 0 || !cyr.MatchString(verbRe.ReplaceAllString(ru, "")) {
		return pattern{}, false
	}
	r, err := regexp.Compile(re.String())
	if err != nil {
		return pattern{}, false
	}
	p := pattern{re: r, key: longest, size: size, quoted: quoted}
	last, next = 0, 1
	for _, m := range verbRe.FindAllStringSubmatchIndex(en, -1) {
		p.out = append(p.out, piece{text: en[last:m[0]]})
		if en[m[0]:m[1]] == "%%" {
			p.out = append(p.out, piece{text: "%"})
		} else {
			arg := next
			if m[4] >= 0 { // %[2]s
				arg, _ = strconv.Atoi(en[m[4]:m[5]])
			}
			next = arg + 1
			if g, ok := group[arg]; ok {
				p.out = append(p.out, piece{arg: g})
			}
		}
		last = m[1]
	}
	p.out = append(p.out, piece{text: en[last:]})
	return p, true
}

// T переводит строку на язык интерфейса: точное совпадение, затем шаблоны. Неизвестное — как есть.
func T(s string) string {
	if !En() || !cyr.MatchString(s) {
		return s
	}
	return translate(s, 0)
}

// Translate — перевод на английский независимо от языка интерфейса.
func Translate(s string) string {
	if !cyr.MatchString(s) {
		return s
	}
	return translate(s, 0)
}

func translate(s string, depth int) string {
	load()
	if v, ok := dict[s]; ok {
		return v
	}
	// пробелы по краям не важны: «Звуков: » = «Звуков:»
	if t := strings.TrimSpace(s); t != s && t != "" {
		if v, ok := dict[t]; ok {
			i := strings.Index(s, t)
			return s[:i] + v + s[i+len(t):]
		}
	}
	cacheMu.Lock()
	v, ok := cache[s]
	cacheMu.Unlock()
	if ok {
		return v
	}
	out := s
	if depth < 3 {
		for _, p := range pats {
			if !strings.Contains(s, p.key) {
				continue
			}
			m := p.re.FindStringSubmatch(s)
			if m == nil {
				continue
			}
			var b strings.Builder
			for _, pc := range p.out {
				if pc.arg == 0 {
					b.WriteString(pc.text)
					continue
				}
				if pc.arg < len(m) {
					a := m[pc.arg]
					if !p.quoted[pc.arg] && cyr.MatchString(a) {
						a = translate(a, depth+1)
					}
					b.WriteString(a)
				}
			}
			out = b.String()
			break
		}
		// составные строки: «Ехидный · слышат все», многострочные сообщения — по частям
		if out == s {
			for _, sep := range []string{"\n", " · ", " — "} {
				if !strings.Contains(s, sep) {
					continue
				}
				parts := strings.Split(s, sep)
				changed := false
				for i, p := range parts {
					if cyr.MatchString(p) {
						if t := translate(p, depth+1); t != p {
							parts[i], changed = t, true
						}
					}
				}
				if changed {
					out = strings.Join(parts, sep)
					break
				}
			}
		}
	}
	cacheMu.Lock()
	if len(cache) > 20000 {
		cache = map[string]string{}
	}
	cache[s] = out
	cacheMu.Unlock()
	return out
}

// Has — есть ли перевод (точный или по шаблону).
func Has(s string) bool {
	return !cyr.MatchString(s) || translate(s, 0) != s
}

// Dict — словарь для интерфейса (JS переводит свои строки сам).
func Dict() map[string]string {
	load()
	return dict
}

// Plural — форма слова для числа n. one/few/many — русские формы («звук», «звука», «звуков»);
// в английском — перевод one для 1 и many для остального.
func Plural(n int, one, few, many string) string {
	if En() {
		if n == 1 || n == -1 {
			return T(one)
		}
		return T(many)
	}
	switch m10, m100 := abs(n)%10, abs(n)%100; {
	case m10 == 1 && m100 != 11:
		return one
	case m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14):
		return few
	}
	return many
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// skipKeys — поля JSON с данными пользователя (названия звуков, фразы, настройки, устройства): их не
// переводим — иначе перевод попадёт обратно в config.json при сохранении.
var skipKeys = map[string]bool{
	"config": true, "settings": true, "devices": true, "capture": true, "playback": true, "mic": true,
	"voice_out": true, "monitor": true, "cable": true, "cable_rec": true, "mic_missing": true,
	"monitor_missing": true, "sounds_dir": true, "recent_apps": true, "path": true, "dir": true, "file": true,
	"files": true, "code": true, "source": true, "exe": true, "device": true, "name_raw": true, "query": true,
	"sounds": true, "reminders": true, "history": true, "text": true, "set": true, "foreground": true,
	"dict": true, "notes": true, "backups": true, "hero": true,
}

// lineKeys — строки журнала («15:04:05 текст»).
var lineKeys = map[string]bool{"logs": true}

// JSON — ответ сервера в JSON с переведёнными строками (кроме данных пользователя, см. skipKeys).
func JSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil || !En() {
		return b, err
	}
	return TranslateJSON(b), nil
}

// TranslateJSON переводит строки в готовом JSON (кроме данных пользователя). Порядок ключей и
// части без русских букв остаются байт в байт.
func TranslateJSON(b []byte) []byte {
	if !En() || !cyr.Match(b) {
		return b
	}
	out, err := walkRaw(b, T)
	if err != nil {
		return b
	}
	return out
}

func walkRaw(raw []byte, tr func(string) string) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !cyr.Match(raw) {
		return raw, nil
	}
	switch raw[0] {
	case '"':
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return json.Marshal(tr(s))
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, e := range arr {
			if i > 0 {
				buf.WriteByte(',')
			}
			v, err := walkRaw(e, tr)
			if err != nil {
				return nil, err
			}
			buf.Write(v)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	case '{':
		dec := json.NewDecoder(bytes.NewReader(raw))
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		buf.WriteByte('{')
		for first := true; dec.More(); first = false {
			t, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, _ := t.(string)
			var v json.RawMessage
			if err := dec.Decode(&v); err != nil {
				return nil, err
			}
			if !first {
				buf.WriteByte(',')
			}
			kb, _ := json.Marshal(k)
			buf.Write(kb)
			buf.WriteByte(':')
			switch {
			case skipKeys[k]:
				buf.Write(v)
				continue
			case lineKeys[k]:
				v, err = walkRaw(v, Line)
			default:
				v, err = walkRaw(v, tr)
			}
			if err != nil {
				return nil, err
			}
			buf.Write(v)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	}
	return raw, nil
}

// lineRe — время в начале строки журнала.
var lineRe = regexp.MustCompile(`^\d{1,2}:\d\d:\d\d `)

// Line переводит строку журнала: время в начале остаётся как есть.
func Line(s string) string {
	if !En() || !cyr.MatchString(s) {
		return s
	}
	if m := lineRe.FindString(s); m != "" {
		return m + T(s[len(m):])
	}
	return T(s)
}

// Detect — язык Windows: русский, украинский, белорусский, казахский → "ru", остальное → "en".
func Detect(uiLang string) string {
	l := strings.ToLower(strings.TrimSpace(uiLang))
	for _, p := range []string{"ru", "uk", "be", "kk"} {
		if l == p || strings.HasPrefix(l, p+"-") || strings.HasPrefix(l, p+"_") {
			return "ru"
		}
	}
	if l == "" {
		return "ru"
	}
	return "en"
}

// Cyrillic — есть ли в строке русские буквы.
func Cyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

var (
	htmlText = regexp.MustCompile(`>([^<>]+)<`)
	htmlAttr = regexp.MustCompile(`\b(title|placeholder|aria-label|alt|content)="([^"]*)"`)
	jsLit    = regexp.MustCompile(`'([^'\\\n]*)'`)
)

// Norm — строка с одиночными пробелами (переносы строк в HTML не важны): так она записана в словаре.
func Norm(s string) string { return strings.Join(strings.Fields(s), " ") }

// HTML переводит страницу: текст между тегами и подписи (title, placeholder…). Перевод вставляется
// как есть (в словаре для HTML нет разметки).
func HTML(s string) string {
	if !En() || !cyr.MatchString(s) {
		return s
	}
	s = strings.Replace(s, `lang="ru"`, `lang="en"`, 1)
	s = htmlText.ReplaceAllStringFunc(s, func(m string) string {
		t := m[1 : len(m)-1]
		n := Norm(t)
		if !cyr.MatchString(n) {
			return m
		}
		lead := t[:len(t)-len(strings.TrimLeft(t, " \t\r\n"))]
		trail := t[len(strings.TrimRight(t, " \t\r\n")):]
		return ">" + lead + T(n) + trail + "<"
	})
	return htmlAttr.ReplaceAllStringFunc(s, func(m string) string {
		p := htmlAttr.FindStringSubmatch(m)
		if !cyr.MatchString(p[2]) {
			return m
		}
		return p[1] + `="` + strings.ReplaceAll(T(Norm(p[2])), `"`, "&quot;") + `"`
	})
}

// JS переводит строки в одинарных кавычках в скрипте (страница пульта — без модулей и словаря).
func JS(b []byte) []byte {
	if !En() || !cyr.Match(b) {
		return b
	}
	return jsLit.ReplaceAllFunc(b, func(m []byte) []byte {
		t := string(m[1 : len(m)-1])
		if !cyr.MatchString(t) {
			return m
		}
		v := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`).Replace(T(t))
		return []byte("'" + v + "'")
	})
}
