package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMaven(t *testing.T) {
	p, e := mavenPath("net.fabricmc:fabric-loader:0.20.1")
	if e != nil || p != "net/fabricmc/fabric-loader/0.20.1/fabric-loader-0.20.1.jar" {
		t.Fatal(p, e)
	}
}
func TestExpand(t *testing.T) {
	raw := []json.RawMessage{json.RawMessage(`"--username"`), json.RawMessage(`"${auth_player_name}"`), json.RawMessage(`{"rules":[{"action":"allow","features":{"is_demo_user":true}}],"value":"--demo"}`)}
	a, e := expand(raw, map[string]string{"auth_player_name": "Player"})
	if e != nil || len(a) != 2 || a[1] != "Player" {
		t.Fatal(a, e)
	}
}
func TestMods(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	p := filepath.Join(src, "chaos.jar")
	f, e := os.Create(p)
	if e != nil {
		t.Fatal(e)
	}
	w := zip.NewWriter(f)
	z, _ := w.Create("fabric.mod.json")
	z.Write([]byte(`{"id":"tiktokchaos"}`))
	w.Close()
	f.Close()
	os.WriteFile(filepath.Join(src, "notes.txt"), []byte("test"), 0600)
	n, e := copyMods(src, dst)
	if n != 1 || e != nil {
		t.Fatal(n, e)
	}
	if e = checkChaos(dst); e != nil {
		t.Fatal(e)
	}
	n, e = copyMods(src, dst)
	if n != 0 || e != nil {
		t.Fatal(n, e)
	}
	if _, e = os.Stat(filepath.Join(dst, "notes.txt")); !os.IsNotExist(e) {
		t.Fatal("non-JAR copied")
	}
}
func TestConfig(t *testing.T) {
	d := t.TempDir()
	c := Config{RAM: 8, CreatorUser: "test"}
	if e := configSave(d, c); e != nil {
		t.Fatal(e)
	}
	if configLoad(d) != c {
		t.Fatal("configuration mismatch")
	}
}
func TestZipTraversal(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "x.zip")
	f, _ := os.Create(p)
	w := zip.NewWriter(f)
	z, _ := w.Create("../bad")
	z.Write([]byte("bad"))
	w.Close()
	f.Close()
	if unzip(p, filepath.Join(d, "out"), nil) == nil {
		t.Fatal("traversal accepted")
	}
}
