package app

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
	"github.com/qwkejkqwje1/dota-voicebox/internal/i18n"
	"github.com/qwkejkqwje1/dota-voicebox/internal/sounds"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// «Читалка»: набранный текст (или сообщение из чата Dota 2) читает голос персонажа — голос Windows
// со своей скоростью, полом и обработкой. Слышит команда (через виртуальный микрофон) и/или вы.

// ReaderVoice — персонаж «Читалки».
type ReaderVoice struct {
	ID    string `json:"id"`
	Emoji string `json:"emoji"`
	Title string `json:"title"`
	Desc  string `json:"desc"`

	gender string     // Male | Female — какой голос Windows подбирать (если голос не выбран явно)
	rate   int        // поправка скорости речи
	preset string     // эффект голоса из каталога (с настройками пользователя)
	fx     []dsp.Spec // или своя цепочка
	live   bool       // текущий эффект голоса и тембр
}

func comp() dsp.Spec {
	return dsp.Spec{Type: "compressor", Threshold: -24, Ratio: 3, AttackMS: 5, ReleaseMS: 120, MakeupDB: 3}
}

func limit() dsp.Spec { return dsp.Spec{Type: "limiter", Threshold: -1} }

// ReaderVoices — персонажи по порядку (← → в меню поверх игры).
var ReaderVoices = []ReaderVoice{
	{ID: "sly", Emoji: "😏", Title: "Ехидный", Desc: "Тянет слова, в нос и с ухмылкой", gender: "Male", rate: -2,
		fx: []dsp.Spec{
			{Type: "pitch", Semitones: 3, WindowMS: 40},
			{Type: "highpass", Freq: 220},
			{Type: "peaking", Freq: 1250, Q: 1.4, DB: 8},
			{Type: "peaking", Freq: 3400, Q: 1, DB: 3},
			{Type: "vibrato", Depth: 0.3, RateHz: 5},
			comp(), limit(),
		}},
	{ID: "vile", Emoji: "🤢", Title: "Мерзкий", Desc: "Гнусавый, булькающий и противный", gender: "Male", rate: -3,
		fx: []dsp.Spec{
			{Type: "pitch", Semitones: -2, WindowMS: 45},
			{Type: "vibrato", Depth: 0.9, RateHz: 6.5},
			{Type: "highpass", Freq: 260},
			{Type: "peaking", Freq: 950, Q: 2.2, DB: 10},
			{Type: "lowpass", Freq: 3800},
			{Type: "drive", Amount: 0.35, Mode: "soft"},
			{Type: "chorus", Depth: 3, RateHz: 0.7, Mix: 0.3},
			comp(), limit(),
		}},
	{ID: "demon", Emoji: "👹", Title: "Демон", Desc: "Низкий рык из преисподней", gender: "Male", rate: -1, preset: "demon"},
	{ID: "robot", Emoji: "🤖", Title: "Робот", Desc: "Металлический голос машины", preset: "robot"},
	{ID: "chipmunk", Emoji: "🐿️", Title: "Бурундук", Desc: "Пискляво и быстро", rate: 2, preset: "chipmunk"},
	{ID: "grandpa", Emoji: "👴", Title: "Дед", Desc: "Старческий дрожащий голос", gender: "Male", rate: -4,
		fx: []dsp.Spec{
			{Type: "pitch", Semitones: -2, WindowMS: 45},
			{Type: "vibrato", Depth: 0.7, RateHz: 5.5},
			{Type: "tremolo", Depth: 0.35, RateHz: 6},
			{Type: "highpass", Freq: 170},
			{Type: "lowpass", Freq: 4200},
			comp(), limit(),
		}},
	{ID: "drunk", Emoji: "🥴", Title: "Пьяный", Desc: "Плывёт и тянет слова", gender: "Male", rate: -3,
		fx: []dsp.Spec{
			{Type: "pitch", Semitones: -1, WindowMS: 50},
			{Type: "vibrato", Depth: 6, RateHz: 0.8},
			{Type: "chorus", Depth: 5, RateHz: 0.4, Mix: 0.35},
			{Type: "lowpass", Freq: 3800},
			limit(),
		}},
	{ID: "bass", Emoji: "🗿", Title: "Басом", Desc: "Низко и солидно", gender: "Male", rate: -1, preset: "deep"},
	{ID: "alien", Emoji: "👽", Title: "Пришелец", Desc: "Плавающий нечеловеческий голос", preset: "alien"},
	{ID: "radio", Emoji: "📻", Title: "Рация", Desc: "Как по рации, с шипением", rate: 1, preset: "radio"},
	{ID: "announcer", Emoji: "🏟️", Title: "Диктор", Desc: "Объявление на весь стадион", gender: "Male", rate: -1, preset: "announcer"},
	{ID: "plain", Emoji: "🙂", Title: "Обычный", Desc: "Голос Windows без обработки"},
	{ID: "live", Emoji: "🎛️", Title: "Мой эффект", Desc: "Текущий эффект голоса и тембр", live: true},
}

// DefaultReaderVoice — персонаж по умолчанию.
const DefaultReaderVoice = "sly"

// FindReaderVoice — персонаж по id.
func FindReaderVoice(id string) (ReaderVoice, bool) {
	for _, v := range ReaderVoices {
		if v.ID == id {
			return v, true
		}
	}
	return ReaderVoice{}, false
}

func readerVoice(id string, cfg *config.Config) ReaderVoice {
	for _, x := range []string{id, cfg.Reader.Voice, DefaultReaderVoice} {
		if v, ok := FindReaderVoice(x); ok {
			return v
		}
	}
	return ReaderVoices[0]
}

// textLang — язык фразы по буквам: кириллица — русский, только латиница — английский.
func textLang(s string) string {
	lat := false
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			return "ru"
		case r < utf8.RuneSelf && unicode.IsLetter(r):
			lat = true
		}
	}
	if lat {
		return "en"
	}
	return ""
}

// readText — текст для чтения: без управляющих символов и лишних пробелов, не длиннее max символов.
func readText(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); max > 0 && len(r) > max {
		s = strings.TrimSpace(string(r[:max])) + "…"
	}
	return s
}

// hasWords — есть ли что читать (буквы или цифры, а не одни знаки).
func hasWords(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// runFX пропускает клип через цепочки эффектов блоками, как в аудиопотоке. В конце — полсекунды тишины
// для хвоста эха (лишняя тишина срезается); значения обрезаются до -1..1.
func runFX(c sounds.Clip, chains []*dsp.Chain) sounds.Clip {
	out := append(append(sounds.Clip(nil), c...), make(sounds.Clip, sr/2)...)
	for i := 0; i < len(out); i += 480 {
		b := out[i:min(i+480, len(out))]
		for _, ch := range chains {
			ch.Process(b)
		}
	}
	n := len(out)
	for n > len(c) && math.Abs(float64(out[n-1])) < 0.001 {
		n--
	}
	out = out[:n]
	for i, x := range out {
		out[i] = max(-1, min(1, x))
	}
	return out
}

// readerChains — обработка голоса персонажа.
func (a *App) readerChains(v ReaderVoice) []*dsp.Chain {
	var specs [][]dsp.Spec
	a.mu.Lock()
	cfg, presets, cur := a.cfg, a.presets, a.preset
	a.mu.Unlock()
	switch {
	case v.live:
		if t := TimbreSpecs(cfg); t != nil {
			specs = append(specs, t)
		}
		if p, ok := presets[cur]; ok {
			specs = append(specs, p)
		}
	case v.preset != "":
		p, ok := presets[v.preset]
		if !ok {
			p = dsp.BuiltinPresets[v.preset]
		}
		specs = append(specs, p)
	case v.fx != nil:
		specs = append(specs, v.fx)
	}
	var out []*dsp.Chain
	for _, s := range specs {
		if ch, err := dsp.BuildChain("reader_"+v.ID, s, sr); err == nil {
			out = append(out, ch)
		} else {
			log.Printf("Читалка: %v", err)
		}
	}
	return out
}

var errNoSpeech = errors.New("синтез речи доступен только в Windows")

// readerOpts — голос Windows для персонажа.
func readerOpts(cfg *config.Config, v ReaderVoice, text string) winapi.SpeakOpts {
	o := winapi.SpeakOpts{Voice: cfg.Reader.Base, Rate: max(-10, min(10, cfg.Reader.Rate+v.rate)), Lang: textLang(text),
		Prefer: cfg.TTSVoice, Temp: true}
	if o.Voice == "" {
		o.Gender = v.gender
	}
	return o
}

// readerClip синтезирует фразу голосом персонажа.
func (a *App) readerClip(text, voice string) (sounds.Clip, ReaderVoice, error) {
	a.mu.Lock()
	sp, cfg := a.speech, a.cfg
	a.mu.Unlock()
	v := readerVoice(voice, cfg)
	if sp == nil {
		return nil, v, errNoSpeech
	}
	c, err := sp.SayWith(text, readerOpts(cfg, v, text))
	if err != nil {
		return nil, v, err
	}
	if ch := a.readerChains(v); len(ch) > 0 {
		c = runFX(c, ch)
	}
	return sounds.Normalize(c), v, nil
}

// ReadItem — прочитанная фраза (история на странице «Читалка»).
type ReadItem struct {
	Text    string `json:"text"`
	Voice   string `json:"voice"`
	Src     string `json:"src"`               // me — набрано | chat — чат Dota 2 | script — скрипт
	Channel string `json:"channel,omitempty"` // чат: all | team
	At      int64  `json:"at"`                // unix мс
}

type readReq struct {
	text, voice, to, src, channel string
}

// readerState — история, последняя фраза (повтор) и очередь сообщений чата.
type readerState struct {
	mu      sync.Mutex
	hist    []ReadItem
	last    sounds.Clip
	lastBus sounds.Bus
	queue   []readReq
	busy    bool
	stop    chan struct{}
	chatN   int
	dropped int
}

const readerHist = 30

func (a *App) remember(r readReq, voice string) {
	a.rd.mu.Lock()
	defer a.rd.mu.Unlock()
	it := ReadItem{Text: r.text, Voice: voice, Src: r.src, Channel: r.channel, At: time.Now().UnixMilli()}
	a.rd.hist = append([]ReadItem{it}, a.rd.hist...)
	if len(a.rd.hist) > readerHist {
		a.rd.hist = a.rd.hist[:readerHist]
	}
	if r.src == "chat" {
		a.rd.chatN++
	}
}

func busOf(to, def string) sounds.Bus {
	if to == "" {
		to = def
	}
	return sounds.ParseBus(to)
}

// Read читает текст голосом «Читалки» сразу (прошлая фраза обрывается). voice — id персонажа ("" — из
// настроек), to — кто слышит: both | voice | monitor ("" — из настроек).
func (a *App) Read(text, voice, to string) error {
	_, err := a.read(readReq{text: text, voice: voice, to: to, src: "me"})
	return err
}

// ReadPreview — проба голоса персонажа: слышите только вы, в историю и «повтор» не попадает.
func (a *App) ReadPreview(text, voice string) error {
	_, err := a.read(readReq{text: text, voice: voice, to: "monitor", src: "preview"})
	return err
}

func (a *App) read(r readReq) (ReaderVoice, error) {
	cfg := a.Config()
	r.text = readText(r.text, 300)
	if !hasWords(r.text) {
		return ReaderVoice{}, fmt.Errorf("пустой текст")
	}
	clip, v, err := a.readerClip(r.text, r.voice)
	if err != nil {
		return v, err
	}
	bus := busOf(r.to, cfg.Reader.To)
	if r.src != "preview" {
		a.remember(r, v.ID)
		a.rd.mu.Lock()
		a.rd.last, a.rd.lastBus = clip, bus
		a.rd.mu.Unlock()
		a.changed()
	}
	return v, a.playClip("reader", clip, float32(cfg.Reader.Volume), sounds.ModeInterrupt, bus)
}

// ReadRepeat — повторить последнюю набранную фразу (без нового синтеза).
func (a *App) ReadRepeat() error {
	a.rd.mu.Lock()
	clip, bus := a.rd.last, a.rd.lastBus
	a.rd.mu.Unlock()
	if clip == nil {
		return fmt.Errorf("ещё ничего не прочитано")
	}
	return a.playClip("reader", clip, float32(a.Config().Reader.Volume), sounds.ModeInterrupt, bus)
}

// ReadStop — замолчать: обрывает фразу и очищает очередь сообщений чата.
func (a *App) ReadStop() {
	a.rd.mu.Lock()
	a.rd.queue = nil
	if a.rd.stop != nil {
		close(a.rd.stop)
	}
	a.rd.stop = make(chan struct{})
	a.rd.busy = false
	a.rd.mu.Unlock()
	a.StopSound("reader")
	a.StopSound("chat")
}

// readQueueMax — сколько сообщений ждут очереди; лишние (самые старые) пропускаются.
const readQueueMax = 3

// queueRead — фраза в очередь (чат и скрипты): фразы не перебивают друг друга.
func (a *App) queueRead(r readReq) {
	a.rd.mu.Lock()
	defer a.rd.mu.Unlock()
	if len(a.rd.queue) >= readQueueMax {
		a.rd.queue = a.rd.queue[1:]
		a.rd.dropped++
	}
	a.rd.queue = append(a.rd.queue, r)
	if a.rd.stop == nil {
		a.rd.stop = make(chan struct{})
	}
	if !a.rd.busy {
		a.rd.busy = true
		go a.readLoop(a.rd.stop)
	}
}

func (a *App) readLoop(stop chan struct{}) {
	for {
		a.rd.mu.Lock()
		if a.rd.stop != stop || len(a.rd.queue) == 0 {
			if a.rd.stop == stop {
				a.rd.busy = false
			}
			a.rd.mu.Unlock()
			return
		}
		r := a.rd.queue[0]
		a.rd.queue = a.rd.queue[1:]
		a.rd.mu.Unlock()

		clip, v, err := a.readerClip(r.text, r.voice)
		if err != nil {
			log.Printf("Читалка: %v", err)
			continue
		}
		select {
		case <-stop:
			return
		default:
		}
		cfg := a.Config()
		a.remember(r, v.ID)
		a.changed()
		if err := a.playClip("chat", clip, float32(cfg.Reader.Volume), sounds.ModeOverlap, busOf(r.to, cfg.Reader.To)); err != nil {
			continue
		}
		d := time.Duration(float64(len(clip))/sr*float64(time.Second)) + 250*time.Millisecond + time.Duration(cfg.PTT.LeadMS)*time.Millisecond
		select {
		case <-time.After(min(d, 40*time.Second)):
		case <-stop:
			return
		}
	}
}

// ReaderInfo — для страницы «Читалка».
type ReaderInfo struct {
	Voices  []ReaderVoice `json:"voices"`
	History []ReadItem    `json:"history"`
	CanSay  bool          `json:"can_say"`
	ChatN   int           `json:"chat_read"`
	Dropped int           `json:"chat_dropped"`
	Repeat  bool          `json:"can_repeat"`
}

func (a *App) ReaderInfo() ReaderInfo {
	a.rd.mu.Lock()
	defer a.rd.mu.Unlock()
	return ReaderInfo{Voices: ReaderVoices, History: append([]ReadItem{}, a.rd.hist...), CanSay: a.CanSay(),
		ChatN: a.rd.chatN, Dropped: a.rd.dropped, Repeat: a.rd.last != nil}
}

// SetReader сохраняет настройки «Читалки».
func (a *App) SetReader(r config.Reader) error {
	if _, ok := FindReaderVoice(r.Voice); !ok {
		r.Voice = DefaultReaderVoice
	}
	if r.Chat.Voice != "" {
		if _, ok := FindReaderVoice(r.Chat.Voice); !ok {
			r.Chat.Voice = ""
		}
	}
	return a.saveQuiet(func(c *config.Config) { c.Reader = r })
}

// cycleReaderVoice — следующий персонаж (клавиша, колесо).
func (a *App) cycleReaderVoice() {
	cur := a.Config().Reader.Voice
	i := 0
	for k, v := range ReaderVoices {
		if v.ID == cur {
			i = (k + 1) % len(ReaderVoices)
		}
	}
	v := ReaderVoices[i]
	a.saveQuiet(func(c *config.Config) { c.Reader.Voice = v.ID })
	a.toast(Toast{Emoji: v.Emoji, Text: fmt.Sprintf("Голос читалки: %s", v.Title), Sub: v.Desc})
}

// toggleChatRead — чтение чата Dota 2 вкл/выкл.
func (a *App) toggleChatRead() {
	on := !a.Config().Reader.Chat.Enabled
	a.saveQuiet(func(c *config.Config) { c.Reader.Chat.Enabled = on })
	t := Toast{Emoji: "💬", Text: "Чтение чата: выкл"}
	if on {
		t.Text = "Чтение чата: вкл"
		if !a.Config().Dota.Enabled {
			t.Sub = "нужен модуль Dota 2 — включите его в VoiceBox"
		}
	}
	a.toast(t)
	if !on {
		a.ReadStop()
	}
}

// readPick — фраза из меню поверх игры: персонаж запоминается для следующего раза.
func (a *App) readPick(text, voice, to string) {
	cfg := a.Config()
	if _, ok := FindReaderVoice(voice); ok && voice != cfg.Reader.Voice {
		a.saveQuiet(func(c *config.Config) { c.Reader.Voice = voice })
	}
	v, err := a.read(readReq{text: text, voice: voice, to: to, src: "me"})
	if err != nil {
		a.toast(Toast{Emoji: "⚠️", Text: "Не удалось прочитать", Sub: err.Error()})
		return
	}
	a.toast(Toast{Emoji: v.Emoji, Text: readText(text, 60), Sub: v.Title + " · " + hears(busOf(to, cfg.Reader.To))})
}

func hears(b sounds.Bus) string {
	switch b {
	case sounds.BusVoice:
		return "слышит команда"
	case sounds.BusMonitor:
		return "слышите только вы"
	}
	return "слышат команда и вы"
}

// ReaderMenu — персонажи и недавние фразы для меню поверх игры.
func (a *App) ReaderMenu() (voices []MenuItem, cur string, hist []string) {
	cur = readerVoice("", a.Config()).ID
	for _, v := range ReaderVoices {
		voices = append(voices, MenuItem{ID: v.ID, Emoji: v.Emoji, Label: i18n.T(v.Title), Find: i18n.T(v.Desc)})
	}
	a.rd.mu.Lock()
	defer a.rd.mu.Unlock()
	seen := map[string]bool{}
	for _, h := range a.rd.hist {
		if h.Src == "me" && !seen[h.Text] && len(hist) < 7 {
			seen[h.Text] = true
			hist = append(hist, h.Text)
		}
	}
	return voices, cur, hist
}

// ---------- чат Dota 2 ----------

// chatWheelMarks — Dota ставит стрелку перед фразами колеса чата.
const chatWheelMarks = "▶►▷▸▹⏵›»>"

// chatText — текст сообщения: без значков-смайликов Dota (U+E000–U+F8FF) и лишних пробелов.
// wheel — фраза колеса чата (стрелка в начале убирается).
func chatText(s string) (text string, wheel bool) {
	s = strings.Map(func(r rune) rune {
		if r >= 0xE000 && r <= 0xF8FF {
			return -1
		}
		return r
	}, s)
	t := readText(s, 0)
	if t == "" {
		return "", false
	}
	if r, n := utf8.DecodeRuneInString(t); strings.ContainsRune(chatWheelMarks, r) {
		return strings.TrimSpace(t[n:]), true
	}
	return t, false
}

// ownPlayerID — номер своего игрока (0–9) по слоту из пакета GSI; -1 — неизвестно.
func ownPlayerID(st *gsi.State) int {
	p := st.Player
	if p == nil {
		return -1
	}
	if s := p.PlayerSlot; s.Ok {
		switch {
		case s.V >= 0 && s.V < 10:
			return s.V
		case s.V >= 128 && s.V < 133:
			return s.V - 128 + 5
		}
	}
	if s := p.TeamSlot; s.Ok && s.V >= 0 && s.V < 5 {
		if strings.EqualFold(p.TeamName, "dire") {
			return s.V + 5
		}
		return s.V
	}
	return -1
}

func chatChannel(t int) string {
	switch t {
	case gsi.ChatAll:
		return "all"
	case gsi.ChatTeam:
		return "team"
	}
	return "other"
}

// onChat — новые сообщения чата (от старых к новым): скриптам — событие chat, вслух — по настройкам.
func (a *App) onChat(msgs []gsi.Event, own int) {
	cfg := a.Config()
	ch := cfg.Reader.Chat
	for _, e := range msgs {
		text, wheel := chatText(e.Message)
		if text == "" {
			continue
		}
		channel := chatChannel(e.ChannelType)
		mine := own >= 0 && e.PlayerID == own
		ally := channel == "team" || (own >= 0 && e.PlayerID >= 0 && (e.PlayerID < 5) == (own < 5))
		a.scripts.Event("chat", map[string]any{"text": text, "channel": channel, "player": e.PlayerID, "own": mine,
			"ally": ally, "wheel": wheel, "time": e.GameTime})
		switch {
		case !ch.Enabled || !hasWords(text):
		case channel == "team" && !ch.Team, channel != "team" && !ch.All:
		case mine && !ch.Own, wheel && !ch.Wheel:
		default:
			a.queueRead(readReq{text: readText(text, ch.MaxLen), voice: ch.Voice, to: ch.To, src: "chat", channel: channel})
		}
	}
}
