// sign — подпись релиза в CI: go run ./tools/sign <tag> <voicebox.exe>  (ключ в $UPDATE_SIGNING_KEY)
// go run ./tools/sign -genkey — создать новую пару ключей.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"github.com/qwkejkqwje1/dota-voicebox/internal/update"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-genkey" {
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		fmt.Println("seed:", base64.StdEncoding.EncodeToString(priv.Seed()))
		fmt.Println("pub: ", base64.StdEncoding.EncodeToString(pub))
		return
	}
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: sign <tag> <file>")
		os.Exit(2)
	}
	seed, err := base64.StdEncoding.DecodeString(os.Getenv("UPDATE_SIGNING_KEY"))
	if err != nil || len(seed) != ed25519.SeedSize {
		fmt.Fprintln(os.Stderr, "UPDATE_SIGNING_KEY не задан или неверен")
		os.Exit(1)
	}
	b, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	sig := ed25519.Sign(priv, update.Message(os.Args[1], b))
	if err := update.Verify(priv.Public().(ed25519.PublicKey), os.Args[1], b, base64.StdEncoding.EncodeToString(sig)); err != nil {
		fmt.Fprintln(os.Stderr, "самопроверка:", err)
		os.Exit(1)
	}
	fmt.Print(base64.StdEncoding.EncodeToString(sig))
}
