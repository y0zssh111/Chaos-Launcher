package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSinglePublisher(t *testing.T) {
	s := Service{File: filepath.Join(t.TempDir(), "state.json"), Bindings: map[string]Binding{}, Results: map[string]Status{}}
	v, e := s.bind("first", "id", "streamer")
	if e != nil || v.State != "verified" {
		t.Fatal(v, e)
	}
	v, e = s.bind("second", "id", "streamer")
	if e != nil || v.State != "denied" || s.status("first").Warnings != 1 {
		t.Fatal(v, e)
	}
	v, e = s.bind("first", "different", "other")
	if e != nil || v.State != "denied" {
		t.Fatal(v, e)
	}
	b, e := os.ReadFile(s.File)
	if e != nil {
		t.Fatal(e)
	}
	var m map[string]Binding
	if json.Unmarshal(b, &m) != nil || m[hash("id")].Warnings != 1 {
		t.Fatal("not persisted")
	}
	if _, ok := m["id"]; ok {
		t.Fatal("raw ID stored")
	}
}
