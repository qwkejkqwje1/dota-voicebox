package app

import (
	"log"
	"sync"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/combo"
	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/keys"
	"github.com/qwkejkqwje1/dota-voicebox/internal/winapi"
)

// Режим «говорю только по кнопке», совместимый с Dota 2 и Discord:
//   - кнопка = кнопка голосового чата игры: держите её как обычно — игра включает войс,
//     VoiceBox открывает микрофон (клавиша не перехватывается, игра её видит);
//   - своя кнопка (например Mouse4): VoiceBox открывает микрофон и сам зажимает кнопку чата игры;
//   - режим «вкл/выкл»: нажал — говорю, нажал ещё раз — тишина.
// Звуки саундборда идут в эфир всегда, независимо от режима.

type talkGate interface {
	SetTalkGate(bool)
	SetTalk(bool)
}

type talkCtl struct {
	mu        sync.Mutex
	mode      string
	key       string
	vk        uint32
	gameVK    uint32
	release   time.Duration
	pressGame bool
	held      bool // кнопка физически нажата
	open      bool // микрофон открыт
	ownGame   bool // мы зажали кнопку игры
	gen       int
}

// TalkStatus — для интерфейса.
type TalkStatus struct {
	Mode string `json:"mode"`
	Key  string `json:"key"`
	Open bool   `json:"open"`
}

func (a *App) applyTalk(cfg *config.Config, gameVK uint32) {
	t := &a.talk
	t.mu.Lock()
	defer t.mu.Unlock()
	mode := cfg.Talk.Mode
	if mode != "ptt" && mode != "toggle" {
		mode = "always"
	}
	key := cfg.Talk.Key
	if key == "" {
		key = cfg.PTT.Key
	}
	var vk uint32
	if mode != "always" {
		if c, err := keys.Parse(key); err != nil || key == "" {
			log.Print("⚠ Режим «по кнопке»: кнопка не назначена — микрофон работает всегда. Назначьте кнопку на вкладке «Пульт».")
			mode = "always"
		} else {
			vk = c.VK
		}
	}
	changed := t.mode != mode || t.vk != vk
	if changed && t.open {
		a.talkCloseLocked()
	}
	t.mode, t.key, t.vk, t.gameVK = mode, key, vk, gameVK
	t.release = time.Duration(max(0, cfg.Talk.ReleaseMS)) * time.Millisecond
	t.pressGame = cfg.Talk.PressGame
	if g, ok := a.out.(talkGate); ok {
		g.SetTalkGate(mode != "always")
		g.SetTalk(t.open)
	}
	if changed && mode != "always" {
		how := map[string]string{"ptt": "пока держите", "toggle": "нажали — вкл, ещё раз — выкл"}[mode]
		log.Printf("🎙 Микрофон в эфир — только по кнопке %s (%s)", key, how)
	}
}

// talkKey вызывается на каждое физическое нажатие/отпускание (не на эмуляцию).
func (a *App) talkKey(ev combo.Event) {
	t := &a.talk
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.mode == "always" || t.vk == 0 || ev.VK != t.vk {
		return
	}
	switch t.mode {
	case "ptt":
		if ev.Down {
			if t.held {
				return // автоповтор
			}
			t.held = true
			t.gen++
			a.talkOpenLocked()
			return
		}
		t.held = false
		t.gen++
		g := t.gen
		if t.release <= 0 {
			a.talkCloseLocked()
			return
		}
		time.AfterFunc(t.release, func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			if t.gen == g && !t.held {
				a.talkCloseLocked()
			}
		})
	case "toggle":
		if !ev.Down {
			t.held = false
			return
		}
		if t.held {
			return
		}
		t.held = true
		a.talkToggleLocked()
	}
}

func (a *App) talkToggleLocked() {
	if a.talk.open {
		a.talkCloseLocked()
	} else {
		a.talkOpenLocked()
	}
}

// TalkToggle — действие для горячей клавиши / скрипта.
func (a *App) TalkToggle() {
	a.talk.mu.Lock()
	defer a.talk.mu.Unlock()
	if a.talk.mode == "always" {
		log.Print("Режим «по кнопке» выключен — включите его на вкладке «Пульт»")
		return
	}
	a.talkToggleLocked()
}

func (a *App) talkOpenLocked() {
	t := &a.talk
	if t.open {
		return
	}
	t.open = true
	if g, ok := a.out.(talkGate); ok {
		g.SetTalk(true)
	}
	// своя кнопка → зажимаем кнопку голосового чата игры
	if t.pressGame && t.gameVK != 0 && t.gameVK != t.vk && !a.pttOwned.Load() && !winapi.IsKeyDown(t.gameVK) {
		if err := winapi.PressKey(t.gameVK, true); err == nil {
			t.ownGame = true
		}
	}
}

func (a *App) talkCloseLocked() {
	t := &a.talk
	if !t.open {
		return
	}
	t.open = false
	if g, ok := a.out.(talkGate); ok {
		g.SetTalk(false)
	}
	if t.ownGame {
		t.ownGame = false
		if a.out.Voice().Active() || a.pending.Load() > 0 {
			// играет звук — кнопку отпустит PTTLoop, когда звук закончится
			a.lastVoice.Store(time.Now().UnixNano())
			a.pttOwned.Store(true)
		} else {
			winapi.PressKey(t.gameVK, false)
		}
	}
}

func (a *App) talkStatus() TalkStatus {
	a.talk.mu.Lock()
	defer a.talk.mu.Unlock()
	return TalkStatus{Mode: a.talk.mode, Key: a.talk.key, Open: a.talk.mode == "always" || a.talk.open}
}
