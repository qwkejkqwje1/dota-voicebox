package winapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindDotaAndInstall(t *testing.T) {
	steam := t.TempDir()
	lib := t.TempDir()
	os.MkdirAll(filepath.Join(steam, "steamapps"), 0o755)
	os.MkdirAll(filepath.Join(lib, "steamapps", "common", "dota 2 beta"), 0o755)
	vdf := `"libraryfolders" { "0" { "path" "` + steam + `" } "1" { "path" "` + lib + `" } }`
	os.WriteFile(filepath.Join(steam, "steamapps", "libraryfolders.vdf"), []byte(vdf), 0o644)
	d, err := FindDota(steam)
	if err != nil {
		t.Fatal(err)
	}
	p, err := InstallGSI(d, "http://127.0.0.1:3000/", "tok")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), `"token" "tok"`) {
		t.Fatal(string(b))
	}
}
