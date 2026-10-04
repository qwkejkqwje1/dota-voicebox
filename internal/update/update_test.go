package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{{"v0.4.0", "v0.3.0", true}, {"v0.3.0", "v0.3.0", false}, {"v0.10.0", "v0.9.9", true}, {"v0.3.1", "v0.4.0", false}, {"v1.0.0", "dev", false}, {"junk", "v0.1.0", false}}
	for _, c := range cases {
		if Newer(c.a, c.b) != c.want {
			t.Fatalf("%s > %s ?", c.a, c.b)
		}
	}
}

func TestCheckDownloadVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	exe := append([]byte("MZ"), []byte("new binary")...)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, Message("v9.0.0", exe)))
	badSig := false
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/" + Repo + "/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v9.0.0","body":"notes","assets":[{"name":"voicebox.exe","browser_download_url":"%s/exe"},{"name":"voicebox.exe.sig","browser_download_url":"%s/sig"}]}`, srv.URL, srv.URL)
		case "/exe":
			w.Write(exe)
		case "/sig":
			if badSig {
				w.Write([]byte(base64.StdEncoding.EncodeToString(make([]byte, 64))))
				return
			}
			w.Write([]byte(sig))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	cur := filepath.Join(dir, "voicebox.exe")
	os.WriteFile(cur, []byte("MZold"), 0o755)
	u := New("v0.4.0", cur)
	u.API, u.Pub = srv.URL, pub
	if _, err := u.Check(); err != nil || !u.Status().Available || u.Status().State != "available" {
		t.Fatalf("%v %+v", err, u.Status())
	}
	if err := u.Download(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(cur + ".new"); string(b) != string(exe) {
		t.Fatal("staged")
	}
	// подменённая подпись — не устанавливается
	badSig = true
	os.Remove(cur + ".new")
	if err := u.Download(); err == nil {
		t.Fatal("плохая подпись принята")
	}
	// подпись от другой версии (попытка «откатить» под видом новой)
	if err := Verify(pub, "v9.0.1", exe, sig); err == nil {
		t.Fatal("подпись другой версии принята")
	}
	// сборка разработчика не обновляется
	d := New("dev", cur)
	d.API, d.Pub = srv.URL, pub
	d.Check()
	if d.Status().Available {
		t.Fatal("dev")
	}
}

func TestRollbackFiles(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "voicebox.exe")
	os.WriteFile(cur, []byte("new"), 0o755)
	os.WriteFile(cur+".old", []byte("old"), 0o755)
	if err := rollbackFiles(cur); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(cur); string(b) != "old" {
		t.Fatal(string(b))
	}
	if b, _ := os.ReadFile(cur + ".bad"); string(b) != "new" {
		t.Fatal("bad copy")
	}
}

func TestEmbeddedKey(t *testing.T) {
	if len(New("v1.0.0", "x").Pub) != ed25519.PublicKeySize {
		t.Fatal("в программу не встроен ключ проверки")
	}
}
