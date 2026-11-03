package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/config"
	"github.com/qwkejkqwje1/dota-voicebox/internal/gsi"
)

// recOut — аудио с записью: «микрофон» отдаёт заранее заданный звук.
type recOut struct {
	fakeOut
	on   bool
	clip []float32
}

func (r *recOut) StartRecord(time.Duration) error { r.on = true; return nil }
func (r *recOut) RecordStatus() (bool, float64, bool) {
	return r.on, float64(len(r.clip)) / sr, false
}
func (r *recOut) StopRecord() []float32 {
	if !r.on {
		return nil
	}
	r.on = false
	return r.clip
}

func gsiState(t *testing.T, js string) *gsi.State {
	t.Helper()
	var s gsi.State
	if err := json.Unmarshal([]byte(js), &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

func TestRecordTrimAndVictory(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, config.DefaultJSON, 0o644)
	out := &recOut{}
	a := New(p, out)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	// запись: полсекунды тишины, секунда голоса (тихий тон), полсекунды тишины
	c := make([]float32, 2*sr)
	for i := sr / 2; i < sr*3/2; i++ {
		c[i] = float32(0.1 * math.Sin(2*math.Pi*220*float64(i)/sr))
	}
	out.clip = c
	if err := a.RecStart(); err != nil {
		t.Fatal(err)
	}
	if st := a.RecStatus(); st["active"] != true {
		t.Fatalf("%v", st)
	}
	w, err := a.RecStop()
	if err != nil {
		t.Fatal(err)
	}
	if w.Duration != 2 || len(w.Peaks) != 800 || math.Abs(w.Sound[0]-0.42) > 0.02 || math.Abs(w.Sound[1]-1.65) > 0.02 || w.Silent {
		t.Fatalf("волна записи: %v–%v из %v", w.Sound[0], w.Sound[1], w.Duration)
	}
	if err := a.RecPlay(w.Sound[0], w.Sound[1], true); err != nil || !out.m.Has("preview") || out.v.Active() {
		t.Fatalf("послушать запись — только себе: %v", err)
	}
	id, err := a.RecSave("Мой клич", w.Sound[0], w.Sound[1], false)
	if err != nil {
		t.Fatal(err)
	}
	s := a.lib.Get(id)
	if id != "moy_klich" || s == nil || s.Emoji != "🎙️" || s.Duration() < 1.1 || s.Duration() > 1.25 {
		t.Fatalf("сохранённая запись %q: %+v", id, s)
	}
	if _, err := a.RecSave("ещё", 0, 0, false); err == nil {
		t.Fatal("запись уже сохранена — второй раз нельзя")
	}

	// обрезка готового звука: файл не меняется, обрезка — в настройках звука
	w2, err := a.SoundWave(id)
	if err != nil || math.Abs(w2.Duration-1.23) > 0.01 {
		t.Fatalf("волна звука: %v %+v", err, w2)
	}
	if err := a.PlayRange(id, 0.1, 0.5); err != nil {
		t.Fatal(err)
	}
	if err := a.TrimSound(id, 0.2, 0.7); err != nil {
		t.Fatal(err)
	}
	if d := a.Config().Sounds[id]; d.Start != 0.2 || d.End != 0.7 {
		t.Fatalf("обрезка не сохранилась: %+v", d)
	}
	if d := a.lib.Get(id).Duration(); math.Abs(d-0.5) > 0.02 {
		t.Fatalf("длина после обрезки: %v", d)
	}
	if err := a.TrimSound(id, 0.5, 0.52); err == nil {
		t.Fatal("слишком короткий кусок")
	}
	if err := a.TrimSound(id, 0, 5); err != nil {
		t.Fatal(err)
	}
	if d := a.Config().Sounds[id]; d.Start != 0 || d.End != 0 {
		t.Fatalf("сброс обрезки: %+v", d)
	}
	if _, err := a.SoundWave("airhorn"); err == nil {
		t.Fatal("встроенный звук обрезать нельзя")
	}

	// звук на победу — без скриптов
	d := a.Config().Dota
	d.Enabled = true
	if err := a.SetDota(d); err != nil {
		t.Fatal(err)
	}
	if err := a.SetEventSound("victory", id, "both"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetEventSound("victory", "nope", ""); err == nil {
		t.Fatal("несуществующий звук")
	}
	if err := a.SetEventSound("Победа!", id, ""); err == nil {
		t.Fatal("неизвестное событие")
	}
	if a.Config().Events["victory"] != id+"@both" || a.Config().Events["low_hp"] != "low_hp" {
		t.Fatalf("события: %v", a.Config().Events)
	}
	a.StopAll()
	a.OnGSI(gsiState(t, `{"map":{"matchid":"5","game_state":"DOTA_GAMERULES_STATE_GAME_IN_PROGRESS","clock_time":1800,"win_team":"none"},"player":{"team_name":"dire"}}`), nil)
	a.OnGSI(gsiState(t, `{"map":{"matchid":"5","game_state":"DOTA_GAMERULES_STATE_POST_GAME","clock_time":1805,"win_team":"dire"},"player":{"team_name":"dire"}}`), nil)
	if !out.v.Has(id) || !out.m.Has(id) {
		t.Fatal("на победу должен играть гимн — и вам, и команде")
	}
	if err := a.SetEventSound("victory", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Config().Events["victory"]; ok {
		t.Fatal("звук с события не снялся")
	}
	a.SetEventSound("defeat", id, "monitor")
	if err := a.DeleteSound(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Config().Events["defeat"]; ok {
		t.Fatal("удалённый звук остался на событии")
	}
}
