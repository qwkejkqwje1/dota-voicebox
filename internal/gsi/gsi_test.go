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
