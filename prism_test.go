package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrismAssetName(t *testing.T) {
	if !portableName.MatchString("PrismLauncher-Windows-MinGW-w64-Portable-11.1.1.zip") || portableName.MatchString("PrismLauncher-Windows-MinGW-arm64-Portable-11.1.1.zip") {
		t.Fatal("wrong architecture")
	}
}
func TestInstanceMigration(t *testing.T) {
	r := t.TempDir()
	p := filepath.Join(r, "game", "saves", "world", "level.dat")
	mkdir(filepath.Dir(p))
	os.WriteFile(p, []byte("world"), 0600)
	if e := initInstance(r); e != nil {
		t.Fatal(e)
	}
	dest := filepath.Join(gameDir(r), "saves", "world", "level.dat")
	b, e := os.ReadFile(dest)
	if e != nil || string(b) != "world" {
		t.Fatal(e, string(b))
	}
	os.WriteFile(dest, []byte("changed"), 0600)
	if e = initInstance(r); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(dest)
	if string(b) != "changed" {
		t.Fatal("overwritten")
	}
	if e = setInstanceOptions(r, Config{RAM: 8}, `C:\Java\bin\java.exe`); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(filepath.Join(instanceDir(r), "instance.cfg"))
	if !strings.Contains(string(b), "MaxMemAlloc=8192") {
		t.Fatal(string(b))
	}
}
func TestReleaseSelection(t *testing.T) {
	r := []Release{{Tag: "launcher-v0.5.0", Assets: []ReleaseAsset{{Name: "TikTokChaosLauncher.exe"}}}, {Tag: "launcher-v0.8.0", Pre: true, Assets: []ReleaseAsset{{Name: "TikTokChaosLauncher.exe"}}}, {Tag: "mod-v1.2.37", Assets: []ReleaseAsset{{Name: "tiktokchaos.jar"}}}}
	if v := findUpdate(r, "owner/repo", "launcher", "0.4.0"); v == nil || v.Version != "0.5.0" {
		t.Fatal(v)
	}
	if v := findUpdate(r, "owner/repo", "mod", "1.2.36-test"); v == nil || v.Version != "1.2.37" {
		t.Fatal(v)
	}
	if !newer("1.2.36", "1.2.36-test") {
		t.Fatal("stable comparison")
	}
}
func testJar(t *testing.T, p, id, v string) {
	t.Helper()
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	z, _ := w.Create("fabric.mod.json")
	z.Write([]byte(`{"id":"` + id + `","version":"` + v + `"}`))
	w.Close()
	f.Close()
}
func TestModBackup(t *testing.T) {
	r := t.TempDir()
	mkdir(modsDir(r))
	old := filepath.Join(modsDir(r), "old.jar")
	other := filepath.Join(modsDir(r), "other.jar")
	testJar(t, old, "tiktokchaos", "1.2.36")
	testJar(t, other, "fabric-api", "1.0.0")
	p := filepath.Join(r, "new.jar")
	testJar(t, p, "tiktokchaos", "1.2.37")
	if e := replaceMod(r, p, "1.2.37"); e != nil {
		t.Fatal(e)
	}
	if e := checkChaos(modsDir(r)); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(other); e != nil {
		t.Fatal(e)
	}
	b, _ := filepath.Glob(filepath.Join(r, "mod-backups", "*", "old.jar"))
	if len(b) != 1 {
		t.Fatal(b)
	}
}
