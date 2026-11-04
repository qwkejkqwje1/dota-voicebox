// Package gsi — приём Dota 2 Game State Integration и генерация игровых событий.
package gsi

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type State struct {
	Auth *struct {
		Token string `json:"token"`
	} `json:"auth"`
	Map *struct {
		Name              string `json:"name"`
		MatchID           string `json:"matchid"`
		GameTime          int    `json:"game_time"`
		ClockTime         int    `json:"clock_time"`
		Daytime           bool   `json:"daytime"`
		NightstalkerNight bool   `json:"nightstalker_night"`
		GameState         string `json:"game_state"`
		Paused            bool   `json:"paused"`
		WinTeam           string `json:"win_team"`
	} `json:"map"`
	Player *struct {
		Name       string `json:"name"`
		TeamName   string `json:"team_name"`
		PlayerSlot OptInt `json:"player_slot"`
		TeamSlot   OptInt `json:"team_slot"`
		Kills      int    `json:"kills"`
		Deaths     int    `json:"deaths"`
		Assists    int    `json:"assists"`
		KillStreak int    `json:"kill_streak"`
		LastHits   int    `json:"last_hits"`
		Gold       int    `json:"gold"`
	} `json:"player"`
	Hero *struct {
		Name          string `json:"name"`
		Level         int    `json:"level"`
		Alive         bool   `json:"alive"`
		HealthPercent int    `json:"health_percent"`
		Smoked        bool   `json:"smoked"`
	} `json:"hero"`
	// Events — провайдер "events" (roshan_killed, aegis_picked_up, bounty_rune_pickup, chat_message…).
	// В каждом пакете — снова последние события матча (обычно от старых к новым).
	Events Events `json:"events"`
}

// Event — событие провайдера "events". Разбирается терпимо: незнакомое событие не ломает весь пакет.
type Event struct {
	GameTime  float64
	EventType string
	Team      string // killed_by_team (roshan_killed)

	// chat_message — сообщение в чате
	PlayerID    int    // кто написал (0–9), -1 — неизвестно
	ChannelType int    // ChatAll — общий чат, ChatTeam — союзники
	Message     string // текст
}

// Каналы чата Dota 2 (channel_type в chat_message).
const (
	ChatAll  = 11
	ChatTeam = 12
)

// EvChat — тип события «сообщение в чате».
const EvChat = "chat_message"

func (e *Event) UnmarshalJSON(b []byte) error {
	var m map[string]any
	*e = Event{PlayerID: -1}
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	e.GameTime, _ = num(m["game_time"])
	e.EventType, _ = m["event_type"].(string)
	e.Team, _ = m["killed_by_team"].(string)
	if v, ok := num(m["player_id"]); ok {
		e.PlayerID = int(v)
	}
	if v, ok := num(m["channel_type"]); ok {
		e.ChannelType = int(v)
	}
	e.Message, _ = m["message"].(string)
	return nil
}

// Events — список событий; не список (или битый) — пусто, а не ошибка всего пакета.
type Events []Event

func (l *Events) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if json.Unmarshal(b, &raw) != nil {
		*l = nil
		return nil
	}
	out := make(Events, 0, len(raw))
	for _, r := range raw {
		var e Event
		e.UnmarshalJSON(r)
		out = append(out, e)
	}
	*l = out
	return nil
}

// OptInt — целое, которого может не быть (или оно пришло строкой).
type OptInt struct {
	V  int
	Ok bool
}

func (o *OptInt) UnmarshalJSON(b []byte) error {
	var v any
	*o = OptInt{}
	if json.Unmarshal(b, &v) == nil {
		if f, ok := num(v); ok {
			*o = OptInt{V: int(f), Ok: true}
		}
	}
	return nil
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

const InProgress = "DOTA_GAMERULES_STATE_GAME_IN_PROGRESS"
const PreGame = "DOTA_GAMERULES_STATE_PRE_GAME"
const PostGame = "DOTA_GAMERULES_STATE_POST_GAME"

// Имена событий, которые можно привязать к звукам в config.json → "events".
const (
	EvGameStart = "game_start" // горн 0:00
	EvPreGame   = "pregame"    // начало пре-гейма
	EvKill      = "kill"
	EvDeath     = "death"
	EvRespawn   = "respawn"
	EvStreak3   = "streak_3"
	EvStreak5   = "streak_5"
	EvStreak10  = "streak_10"
	EvLowHP     = "low_hp"
	EvLevel6    = "level_6"
	EvSmoked    = "smoked"
	EvDay       = "day"
	EvNight     = "night"
	EvVictory   = "victory"
	EvDefeat    = "defeat"
)

// team — настоящая команда (radiant, dire), а не "none" или зритель.
func team(t string) bool {
	t = strings.ToLower(t)
	return t != "" && t != "none" && t != "spectator"
}

// Diff сравнивает два состояния и возвращает события.
func Diff(prev, cur *State, lowHP int) []string {
	if cur == nil || cur.Map == nil {
		return nil
	}
	var ev []string
	if prev == nil || prev.Map == nil || prev.Map.MatchID != cur.Map.MatchID {
		return nil // первый пакет матча: только запоминаем
	}
	pm, cm := prev.Map, cur.Map
	if pm.GameState != cm.GameState {
		switch cm.GameState {
		case InProgress:
			ev = append(ev, EvGameStart)
		case PreGame:
			ev = append(ev, EvPreGame)
		}
	}
	// конец матча: победитель приходит вместе с POST_GAME или следующим пакетом — событие один раз
	if cm.GameState == PostGame && team(cm.WinTeam) && cur.Player != nil && team(cur.Player.TeamName) &&
		(pm.GameState != PostGame || !team(pm.WinTeam)) {
		if strings.EqualFold(cm.WinTeam, cur.Player.TeamName) {
			ev = append(ev, EvVictory)
		} else {
			ev = append(ev, EvDefeat)
		}
	}
	if cm.GameState == InProgress && pm.Daytime != cm.Daytime {
		if cm.Daytime {
			ev = append(ev, EvDay)
		} else {
			ev = append(ev, EvNight)
		}
	}
	if p, c := prev.Player, cur.Player; p != nil && c != nil {
		if c.Kills > p.Kills {
			ev = append(ev, EvKill)
		}
		if c.Deaths > p.Deaths {
			ev = append(ev, EvDeath)
		}
		for _, s := range []struct {
			n  int
			ev string
		}{{3, EvStreak3}, {5, EvStreak5}, {10, EvStreak10}} {
			if p.KillStreak < s.n && c.KillStreak >= s.n {
				ev = append(ev, s.ev)
			}
		}
	}
	if p, c := prev.Hero, cur.Hero; p != nil && c != nil {
		if !p.Alive && c.Alive {
			ev = append(ev, EvRespawn)
		}
		if c.Alive && p.HealthPercent >= lowHP && c.HealthPercent < lowHP && c.HealthPercent > 0 {
			ev = append(ev, EvLowHP)
		}
		if p.Level < 6 && c.Level >= 6 {
			ev = append(ev, EvLevel6)
		}
		if !p.Smoked && c.Smoked {
			ev = append(ev, EvSmoked)
		}
	}
	return ev
}

// Server принимает POST от Dota 2.
type Server struct {
	Addr    string
	Token   string
	OnState func(st *State, raw []byte)

	mu  sync.Mutex
	srv *http.Server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	w.WriteHeader(http.StatusOK)
	if err != nil {
		return
	}
	var st State
	if err := json.Unmarshal(body, &st); err != nil {
		log.Printf("GSI: плохой JSON: %v", err)
		return
	}
	if s.Token != "" && (st.Auth == nil || st.Auth.Token != s.Token) {
		return
	}
	s.OnState(&st, body)
}

func (s *Server) Run() error {
	return http.ListenAndServe(s.Addr, s)
}

// Start начинает слушать адрес в фоне (ошибка — если порт занят).
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()
	go srv.Serve(ln)
	return nil
}

// Close останавливает сервер, запущенный через Start.
func (s *Server) Close() {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
}
