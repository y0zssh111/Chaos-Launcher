package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSwapExecutable(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "my launcher.exe")
			stageDir := filepath.Join(dir, ".chaos-update-test")
			if err := os.Mkdir(stageDir, 0700); err != nil {
				t.Fatal(err)
			}
			stage := filepath.Join(stageDir, "new.exe")
			backup := filepath.Join(stageDir, "previous.exe")
			if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stage, []byte("new"), 0600); err != nil {
				t.Fatal(err)
			}
			err := swapExecutable(target, stage, backup, func() error {
				b, e := os.ReadFile(target)
				if e != nil || string(b) != "new" {
					t.Fatal("replacement missing", e)
				}
				if fail {
					return errors.New("start failed")
				}
				return nil
			})
			if (err != nil) != fail {
				t.Fatal(err)
			}
			want := "new"
			if fail {
				want = "old"
			}
			b, e := os.ReadFile(target)
			if e != nil || string(b) != want {
				t.Fatal(string(b), e)
			}
		})
	}
}
func TestSwapMissingStageRestores(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "launcher.exe")
	if e := os.WriteFile(p, []byte("old"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := swapExecutable(p, filepath.Join(d, "missing.exe"), filepath.Join(d, "previous.exe"), func() error { t.Fatal("must not start"); return nil }); e == nil {
		t.Fatal("expected error")
	}
	b, e := os.ReadFile(p)
	if e != nil || string(b) != "old" {
		t.Fatal(string(b), e)
	}
}
