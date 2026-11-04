// Game events and hero state (example)
//
// on("kill" | "death" | "respawn" | "streak_3" | "streak_5" | "streak_10" |
//    "low_hp" | "level_6" | "smoked" | "day" | "night" | "game_start" |
//    "pregame" | "victory" | "defeat" | "new_game" | "rosh_killed" |
//    "clock" (every game second) | "state" (every GSI packet), fn)
//
// game.clock, game.seconds, game.hero, game.paused, game.daytime, game.state,
// game.raw — the full JSON from Dota (hero, player, abilities, items...).

on("death", () => {
  if (game.clock !== null && game.clock > 30 * 60) {
    say("Late game, don't die!", { to: "me" });
  }
});

on("night", () => {
  if (game.hero === "night_stalker") play("danger", { to: "me" });
});

// raw data example: a reminder when your ultimate is off cooldown
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
  notify("Now " + (game.clock === null ? "no game" : fmt(game.clock)) + ", time source: " + game.clockSource);
});
