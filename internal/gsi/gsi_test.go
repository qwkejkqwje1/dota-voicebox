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
