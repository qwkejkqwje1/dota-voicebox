// События игры и состояние героя (пример)
//
// on("kill" | "death" | "respawn" | "streak_3" | "streak_5" | "streak_10" |
//    "low_hp" | "level_6" | "smoked" | "day" | "night" | "game_start" |
//    "pregame" | "victory" | "defeat" | "new_game" | "rosh_killed" |
//    "clock" (каждую игровую секунду) | "state" (каждый пакет GSI), fn)
//
// game.clock, game.seconds, game.hero, game.paused, game.daytime, game.state,
// game.raw — полный JSON от Dota (hero, player, abilities, items...).

on("death", () => {
  if (game.clock !== null && game.clock > 30 * 60) {
    say("Поздняя игра, не умирай!", { to: "me" });
  }
});

on("night", () => {
  if (game.hero === "night_stalker") play("danger", { to: "me" });
});

// пример с сырыми данными: напомнить про ульт, когда он откатился
let ultReady = false;
on("state", () => {
  const r = game.raw;
  if (!r || !r.abilities) return;
  for (const k in r.abilities) {
    const a = r.abilities[k];
    if (a.ultimate && a.level > 0) {
      if (a.can_cast && !ultReady) play("beep", { to: "me" });
      ultReady = a.can_cast;
    }
  }
});

hotkey("Ctrl+Shift+T", () => {
  notify("Сейчас " + (game.clock === null ? "нет игры" : fmt(game.clock)) + ", источник времени: " + game.clockSource);
});
