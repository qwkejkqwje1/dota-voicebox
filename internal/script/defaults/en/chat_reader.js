// Reading Dota 2 chat in different voices (example)
//
// Simple chat reading is turned on on the “Reader” page, no scripts needed. This example is for when
// you want it your way: allies in one voice, enemies in another, your own messages and the chat wheel skipped.
//
// on("chat", (m) => …) — a new message in the match chat (needs the Dota 2 module):
//   m.text — the text, m.channel — "all" (to everyone) | "team" (to the team), m.player — player number 0–9,
//   m.own — your message, m.ally — an ally, m.wheel — a chat wheel phrase, m.time — game time.
// read(text, { voice, to }) — read in a Reader voice (phrases are queued and don't interrupt each other):
//   voice — sly, vile, demon, robot, chipmunk, grandpa, drunk, bass, alien,
//   radio, announcer, plain, live; to — "me" (only you) | "team" (the team) | "both".

on("chat", (m) => {
  if (m.own || m.wheel) return; // skip your own messages and the chat wheel
  if (m.text.length > 100) return; // and walls of text
  const voice = m.ally ? "announcer" : "vile";
  read(m.text, { voice: voice, to: "me" }); // only you hear it
});
