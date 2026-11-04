package app

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/dsp"
)

func TestReaderText(t *testing.T) {
	for in, want := range map[string]string{"привет": "ru", "gg мид": "ru", "hello": "en", "123 !!": "", "ñ": ""} {
		if got := textLang(in); got != want {
			t.Errorf("textLang(%q) = %q, want %q", in, got, want)
		}
	}
	if s := readText("  мид\n\tss  ", 0); s != "мид ss" {
		t.Fatalf("readText: %q", s)
	}
	if s := readText("раз два три", 7); s != "раз два…" {
		t.Fatalf("обрезка: %q", s)
	}
	if hasWords("!!! ???") || hasWords("") || !hasWords("gg") || !hasWords("1") {
		t.Fatal("hasWords")
	}
	for in, want := range map[string]struct {
		text  string
		wheel bool
	}{
		"\ue001 gg  wp \ue002": {"gg wp", false},
		"▶ Хорошая игра":       {"Хорошая игра", true},
		"\ue010":               {"", false},
		"мид ss":               {"мид ss", false},
	} {
		if text, wheel := chatText(in); text != want.text || wheel != want.wheel {
			t.Errorf("chatText(%q) = %q %v", in, text, wheel)
		}
	}
	for js, want := range map[string]int{
		`{"player":{"player_slot":2}}`:                     2,
		`{"player":{"player_slot":130}}`:                   7,
		`{"player":{"team_slot":3,"team_name":"dire"}}`:    8,
		`{"player":{"team_slot":1,"team_name":"radiant"}}`: 1,
		`{"player":{"name":"x"}}`:                          -1,
		`{}`:                                               -1,
	} {
		if got := ownPlayerID(gsiState(t, js)); got != want {
			t.Errorf("ownPlayerID(%s) = %d, want %d", js, got, want)
		}
	}
	if chatChannel(11) != "all" || chatChannel(12) != "team" || chatChannel(0) != "other" {
		t.Fatal("chatChannel")
	}
}

func TestReaderVoices(t *testing.T) {
	a, _ := newApp(t, true)
	seen := map[string]bool{}
	for _, v := range ReaderVoices {
		if seen[v.ID] || v.Emoji == "" || v.Title == "" || v.Desc == "" {
			t.Fatalf("персонаж %+v", v)
		}
		seen[v.ID] = true
		if v.preset != "" && dsp.BuiltinPresets[v.preset] == nil {
			t.Fatalf("%s: нет пресета %q", v.ID, v.preset)
		}
		c, got, err := a.readerClip("Привет, как дела у команды", v.ID)
		if err != nil || got.ID != v.ID || len(c) < sr/4 {
			t.Fatalf("%s: %v, %d отсчётов", v.ID, err, len(c))
		}
		peak := 0.0
		for _, x := range c {
			if math.IsNaN(float64(x)) || x < -1 || x > 1 {
				t.Fatalf("%s: отсчёт %v", v.ID, x)
			}
			peak = max(peak, math.Abs(float64(x)))
		}
		if peak < 0.1 {
			t.Fatalf("%s: слишком тихо (%v)", v.ID, peak)
		}
	}
	if !seen[DefaultReaderVoice] || !seen["vile"] || !seen["sly"] {
		t.Fatal("нет ехидного или мерзкого")
	}
	cfg := a.Config()
	v, _ := FindReaderVoice("grandpa")
	if o := readerOpts(cfg, v, "hello"); o.Gender != "Male" || o.Lang != "en" || o.Rate != -4 || o.Voice != "" {
		t.Fatalf("readerOpts: %+v", o)
	}
	c := *cfg
	c.Reader.Base, c.Reader.Rate = "Microsoft Irina", -8
	if o := readerOpts(&c, v, "привет"); o.Gender != "" || o.Voice != "Microsoft Irina" || o.Rate != -10 || o.Lang != "ru" {
		t.Fatalf("readerOpts с голосом: %+v", o)
	}
	if readerVoice("nope", cfg).ID != DefaultReaderVoice {
		t.Fatal("неизвестный персонаж — из настроек")
	}
}

func TestReaderRead(t *testing.T) {
	a, out := newApp(t, true)
	if err := a.ReadRepeat(); err == nil {
		t.Fatal("повторять ещё нечего")
	}
	for _, s := range []string{"", "   ", "!!! ?"} {
		if err := a.Read(s, "", ""); err == nil {
			t.Fatalf("пустой текст %q", s)
		}
	}
	if err := a.Read("Ну   привет", "", ""); err != nil {
		t.Fatal(err)
	}
	if !out.v.Has("reader") || !out.m.Has("reader") {
		t.Fatal("по умолчанию слышат и команда, и вы")
	}
	info := a.ReaderInfo()
	if len(info.History) != 1 || info.History[0].Text != "Ну привет" || info.History[0].Voice != "sly" || info.History[0].Src != "me" || !info.Repeat || !info.CanSay {
		t.Fatalf("история: %+v", info)
	}
	a.StopAll()
	if err := a.Read("только мне", "vile", "monitor"); err != nil {
		t.Fatal(err)
	}
	if out.v.Has("reader") || !out.m.Has("reader") {
		t.Fatal("monitor — слышите только вы")
	}
	a.StopAll()
	if err := a.ReadRepeat(); err != nil || out.v.Has("reader") || !out.m.Has("reader") {
		t.Fatalf("повтор — туда же: %v", err)
	}
	a.StopAll()
	if err := a.ReadPreview("проба", "demon"); err != nil || out.v.Has("reader") || !out.m.Has("reader") {
		t.Fatalf("проба голоса: %v", err)
	}
	if h := a.ReaderInfo().History; len(h) != 2 || h[0].Text != "только мне" || h[0].Voice != "vile" {
		t.Fatalf("проба не попадает в историю: %+v", h)
	}
	a.ReadStop()
	if out.m.Has("reader") {
		t.Fatal("замолчать")
	}

	// меню поверх игры: персонаж запоминается
	a.StopAll()
	a.MenuPick("readme:robot", "Скажи гав")
	if a.Config().Reader.Voice != "robot" || out.v.Has("reader") || !out.m.Has("reader") {
		t.Fatalf("readme: %q", a.Config().Reader.Voice)
	}
	a.MenuPick("read:vile", "Скажи гав")
	a.MenuPick("read", "Ещё раз")
	if h := a.ReaderInfo().History; a.Config().Reader.Voice != "vile" || h[0].Voice != "vile" || !out.v.Has("reader") {
		t.Fatalf("read: %+v", h[0])
	}
	_, cur, hist := a.ReaderMenu()
	if cur != "vile" || strings.Join(hist, "|") != "Ещё раз|Скажи гав|только мне|Ну привет" {
		t.Fatalf("недавние фразы: %q %q", cur, hist)
	}
	a.Do("reader:next")
	if a.Config().Reader.Voice != "demon" {
		t.Fatalf("следующий персонаж: %q", a.Config().Reader.Voice)
	}
	a.Do("chat_read")
	if !a.Config().Reader.Chat.Enabled {
		t.Fatal("чтение чата включается")
	}
	a.Do("chat_read")
	if a.Config().Reader.Chat.Enabled {
		t.Fatal("и выключается")
	}

	r := a.Config().Reader
	r.Voice, r.Chat.Voice, r.Chat.MaxLen = "nope", "bad", 5
	if err := a.SetReader(r); err != nil {
		t.Fatal(err)
	}
	if r := a.Config().Reader; r.Voice != DefaultReaderVoice || r.Chat.Voice != "" || r.Chat.MaxLen != 20 {
		t.Fatalf("проверка настроек: %+v", r)
	}
}

func TestReaderNoSpeech(t *testing.T) {
	a, _ := newApp(t, false)
	if err := a.Read("привет", "", ""); err == nil || a.ReaderInfo().CanSay {
		t.Fatal("без синтеза речи читать нечем")
	}
}

func waitChat(t *testing.T, a *App, n int) {
	t.Helper()
	for i := 0; i < 500; i++ {
		a.rd.mu.Lock()
		got, busy := a.rd.chatN, a.rd.busy
		a.rd.mu.Unlock()
		if got >= n && !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("прочитано %d сообщений чата из %d", a.ReaderInfo().ChatN, n)
}

func TestReaderChat(t *testing.T) {
	a, out := newApp(t, true)
	r := a.Config().Reader
	r.Chat.Enabled = true
	if err := a.SetReader(r); err != nil {
		t.Fatal(err)
	}
	pkt := func(clock int, events string) {
		a.OnGSI(gsiState(t, `{"map":{"matchid":"7","game_state":"DOTA_GAMERULES_STATE_GAME_IN_PROGRESS","clock_time":`+
			strconv.Itoa(clock)+`},"player":{"team_name":"radiant","player_slot":2},"events":[`+events+`]}`), nil)
	}
	old := `{"game_time":590,"event_type":"chat_message","player_id":7,"channel_type":11,"message":"старое"}`
	pkt(600, old) // первый пакет: старые сообщения не читаем
	time.Sleep(50 * time.Millisecond)
	if n := a.ReaderInfo().ChatN; n != 0 {
		t.Fatalf("старое сообщение прочитано: %d", n)
	}
	msgs := old + `,
	{"game_time":598,"event_type":"chat_message","player_id":4,"channel_type":12,"message":"мид ss"},
	{"game_time":595,"event_type":"chat_message","player_id":2,"channel_type":11,"message":"я свой"},
	{"game_time":596,"event_type":"chat_message","player_id":3,"channel_type":11,"message":"▶ Хорошая игра"},
	{"game_time":597,"event_type":"chat_message","player_id":7,"channel_type":11,"message":"gg \ue001 ez"},
	{"game_time":599,"event_type":"chat_message","player_id":5,"channel_type":11,"message":"\ue002"},
	{"game_time":599,"event_type":"roshan_killed","killed_by_team":"dire"}`
	pkt(601, msgs)
	waitChat(t, a, 2)
	h := a.ReaderInfo().History
	if len(h) != 2 || h[0].Text != "мид ss" || h[0].Channel != "team" || h[1].Text != "gg ez" || h[1].Channel != "all" || h[1].Src != "chat" {
		t.Fatalf("прочитано из чата: %+v", h)
	}
	if !out.m.Has("chat") || out.v.Has("chat") {
		t.Fatal("чат по умолчанию слышите только вы")
	}
	pkt(602, msgs) // те же сообщения ещё раз — не повторяем
	a.rd.mu.Lock()
	busy := a.rd.busy
	a.rd.mu.Unlock()
	if busy || a.ReaderInfo().ChatN != 2 {
		t.Fatal("повторное сообщение прочитано ещё раз")
	}
	// своё сообщение и колесо чата — если включено
	r = a.Config().Reader
	r.Chat.Own, r.Chat.Wheel, r.Chat.Team = true, true, false
	a.SetReader(r)
	pkt(603, `{"game_time":603,"event_type":"chat_message","player_id":2,"channel_type":11,"message":"▶ Хорошая игра"},
	{"game_time":603,"event_type":"chat_message","player_id":1,"channel_type":12,"message":"тиммейт"}`)
	waitChat(t, a, 3)
	if h := a.ReaderInfo().History; len(h) != 3 || h[0].Text != "Хорошая игра" {
		t.Fatalf("колесо чата: %+v", h)
	}
	// выключено — не читаем
	r.Chat.Enabled = false
	a.SetReader(r)
	pkt(604, `{"game_time":604,"event_type":"chat_message","player_id":8,"channel_type":11,"message":"ку"}`)
	time.Sleep(30 * time.Millisecond)
	if a.ReaderInfo().ChatN != 3 {
		t.Fatal("чтение чата выключено")
	}
}
