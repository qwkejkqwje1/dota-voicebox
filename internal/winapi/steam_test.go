package winapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if _, err := InstallGSI(d, "http://127.0.0.1:3000/", "tok"); err != nil {
		t.Fatal(err)
	}
	if !GSIInstalled(d, "http://127.0.0.1:3000/", "tok") || GSIInstalled(d, "http://x/", "tok") {
		t.Fatal("GSIInstalled")
	}
}

func TestFindVoiceKey(t *testing.T) {
	steam := t.TempDir()
	old := filepath.Join(steam, "userdata", "111", "570", "remote", "cfg")
	cur := filepath.Join(steam, "userdata", "222", "570", "remote", "cfg")
	os.MkdirAll(old, 0o755)
	os.MkdirAll(cur, 0o755)
	os.WriteFile(filepath.Join(old, "user_keys_0_slot0.vcfg"), []byte(`"config"{"bindings"{"t" "+voicerecord"}}`), 0o644)
	p := filepath.Join(cur, "user_keys_0_slot0.vcfg")
	os.WriteFile(p, []byte("\"config\"\n{\n\t\"bindings\"\n\t{\n\t\t\"MOUSE4\"\t\t\"+voicerecord\"\n\t\t\"q\"\t\t\"dota_ability_execute 0\"\n\t}\n}"), 0o644)
	future := time.Now().Add(time.Hour)
	os.Chtimes(p, future, future)
	k, f, err := FindVoiceKey(steam, "")
	if err != nil || k != "Mouse4" || f != p {
		t.Fatalf("%q %q %v", k, f, err)
	}
	if n, ok := SourceKeyName("KP_END"); !ok || n != "Num1" {
		t.Fatal(n)
	}
}

func TestLaunchOptions(t *testing.T) {
	steam := t.TempDir()
	dir := filepath.Join(steam, "userdata", "1", "config")
	os.MkdirAll(dir, 0o755)
	src := "\"UserLocalConfigStore\"\n{\n\t\"Software\"\n\t{\n\t\t\"Valve\"\n\t\t{\n\t\t\t\"Steam\"\n\t\t\t{\n\t\t\t\t\"apps\"\n\t\t\t\t{\n\t\t\t\t\t\"570\"\n\t\t\t\t\t{\n\t\t\t\t\t\t\"LaunchOptions\"\t\t\"-novid\"\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n\t\"friends\"\n\t{\n\t\t\"x\"\t\t\"a \\\"q\\\" b\"\n\t}\n}\n"
	f := filepath.Join(dir, "localconfig.vdf")
	os.WriteFile(f, []byte(src), 0o644)
	if has, err := HasLaunchOption(steam); err != nil || has {
		t.Fatal(has, err)
	}
	if n, err := AddLaunchOption(steam); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	b, _ := os.ReadFile(f)
	if !strings.Contains(string(b), `"-novid -gamestateintegration"`) || !strings.Contains(string(b), `"a \"q\" b"`) {
		t.Fatal(string(b))
	}
	if has, _ := HasLaunchOption(steam); !has {
		t.Fatal("не добавилось")
	}
	// повторно — без изменений
	if n, _ := AddLaunchOption(steam); n != 0 {
		t.Fatal("повторное добавление")
	}
}

func TestVDFRoundtrip(t *testing.T) {
	src := "\"a\"\n{\n\t\"b\"\t\t\"1\"\n\t\"c\"\n\t{\n\t}\n}\n"
	root, err := ParseVDF(src)
	if err != nil || root.String() != src {
		t.Fatalf("%v\n%s", err, root.String())
	}
}
