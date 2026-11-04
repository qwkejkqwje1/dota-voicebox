// My timings: your own reminders by game time (example)
//
// at("6:00", fn)               — once at 6:00
// at("6:00", fn, {before: 10}) — 10 seconds before 6:00
// every("7:00", fn, {from: "7:00", until: "35:00", before: 15}) — repeat
// Game time comes from Dota (GSI) and is smoothed between packets;
// without GSI, press “Horn 0:00” on the “Dota 2” page and the clock runs manually.

at("0:00", () => notify("Horn! Good luck, have fun"));

// lotuses every 3 minutes from 3:00, a warning 10 seconds ahead
every("3:00", (t) => {
  play("beep", { to: "me" });
  notify("Lotus at " + fmt(t));
}, { from: "3:00", until: "12:00", before: 10 });

// your own reminder, just for you
at("10:00", () => say("Check if it's time to buy BKB", { to: "me" }), { before: 5 });

// Tormentor spawns at 20:00
at("20:00", () => say("Tormentor in 30 seconds", { to: "team" }), { before: 30 });
