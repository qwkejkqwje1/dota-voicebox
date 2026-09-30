// Package gsi — приём Dota 2 Game State Integration и генерация игровых событий.
package gsi

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
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
		case PostGame:
			if cur.Player != nil && cm.WinTeam != "" && cm.WinTeam != "none" {
				if cm.WinTeam == cur.Player.TeamName {
					ev = append(ev, EvVictory)
				} else {
					ev = append(ev, EvDefeat)
				}
			}
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
	OnState func(*State)
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
	s.OnState(&st)
}

func (s *Server) Run() error {
	return http.ListenAndServe(s.Addr, s)
}
