package gsi

import (
	"encoding/json"
	"testing"
)

func st(js string) *State {
	var s State
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		panic(err)
	}
	return &s
}

func TestDiff(t *testing.T) {
	a := st(`{"map":{"matchid":"1","game_state":"DOTA_GAMERULES_STATE_GAME_IN_PROGRESS","daytime":true},"player":{"deaths":0,"kills":2,"kill_streak":2},"hero":{"alive":true,"health_percent":50,"level":5}}`)
	b := st(`{"map":{"matchid":"1","game_state":"DOTA_GAMERULES_STATE_GAME_IN_PROGRESS","daytime":false},"player":{"deaths":0,"kills":3,"kill_streak":3},"hero":{"alive":true,"health_percent":20,"level":6}}`)
	got := map[string]bool{}
	for _, e := range Diff(a, b, 25) {
		got[e] = true
	}
	for _, w := range []string{EvNight, EvKill, EvStreak3, EvLowHP, EvLevel6} {
		if !got[w] {
			t.Errorf("нет события %s: %v", w, got)
		}
	}
	if got[EvDeath] {
		t.Error("лишняя смерть")
	}
}

func TestVictoryDefeat(t *testing.T) {
	const run = `{"map":{"matchid":"7","game_state":"DOTA_GAMERULES_STATE_GAME_IN_PROGRESS","win_team":"none"},"player":{"team_name":"radiant"}}`
	evs := func(a, b string) []string { return Diff(st(a), st(b), 25) }
	has := func(l []string, e string) bool {
		for _, x := range l {
			if x == e {
				return true
			}
		}
		return false
	}
	// победитель — сразу с концом матча
	won := `{"map":{"matchid":"7","game_state":"DOTA_GAMERULES_STATE_POST_GAME","win_team":"radiant"},"player":{"team_name":"radiant"}}`
	if e := evs(run, won); !has(e, EvVictory) || has(e, EvDefeat) {
		t.Fatalf("победа: %v", e)
	}
	if e := evs(won, won); has(e, EvVictory) {
		t.Fatalf("победа второй раз: %v", e)
	}
	// победитель — следующим пакетом
	end := `{"map":{"matchid":"7","game_state":"DOTA_GAMERULES_STATE_POST_GAME","win_team":"none"},"player":{"team_name":"radiant"}}`
	lost := `{"map":{"matchid":"7","game_state":"DOTA_GAMERULES_STATE_POST_GAME","win_team":"dire"},"player":{"team_name":"radiant"}}`
	if e := evs(run, end); has(e, EvVictory) || has(e, EvDefeat) {
		t.Fatalf("победителя ещё нет: %v", e)
	}
	if e := evs(end, lost); !has(e, EvDefeat) || has(e, EvVictory) {
		t.Fatalf("поражение: %v", e)
	}
	// зритель — ни победы, ни поражения
	spec := `{"map":{"matchid":"7","game_state":"DOTA_GAMERULES_STATE_POST_GAME","win_team":"dire"},"player":{}}`
	if e := evs(run, spec); has(e, EvVictory) || has(e, EvDefeat) {
		t.Fatalf("зритель: %v", e)
	}
}

func TestEventsTolerant(t *testing.T) {
	s := st(`{"map":{"matchid":"1"},"events":[
		{"game_time": 812.5, "event_type": "chat_message", "player_id": 3, "channel_type": 12, "message": "мид ss"},
		{"game_time": "900", "event_type": "roshan_killed", "killed_by_team": "dire"},
		{"event_type": "chat_message", "player_id": "7", "message": 5},
		"мусор", {"event_type": {"x": 1}}],
		"player":{"team_name":"dire","player_slot":"132","team_slot":4}}`)
	if len(s.Events) != 5 {
		t.Fatalf("событий: %d", len(s.Events))
	}
	e := s.Events[0]
	if e.EventType != EvChat || e.GameTime != 812.5 || e.PlayerID != 3 || e.ChannelType != ChatTeam || e.Message != "мид ss" {
		t.Fatalf("чат: %+v", e)
	}
	if r := s.Events[1]; r.GameTime != 900 || r.Team != "dire" || r.PlayerID != -1 {
		t.Fatalf("рошан: %+v", r)
	}
	if c := s.Events[2]; c.PlayerID != 7 || c.Message != "" {
		t.Fatalf("странный чат: %+v", c)
	}
	if !s.Player.PlayerSlot.Ok || s.Player.PlayerSlot.V != 132 || s.Player.TeamSlot.V != 4 {
		t.Fatalf("слоты: %+v %+v", s.Player.PlayerSlot, s.Player.TeamSlot)
	}
	// events — не список: пакет всё равно разбирается
	if s2 := st(`{"map":{"matchid":"2"},"events":{"a":1}}`); s2.Map == nil || len(s2.Events) != 0 {
		t.Fatal("events-объект сломал пакет")
	}
}
