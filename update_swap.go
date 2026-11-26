package main

import (
	"fmt"
	"os"
)

// Keep the installed filename unchanged; restore old executable if replacement
// or starting the new process fails. The caller owns the staging directory.
func swapExecutable(target, staged, backup string, start func() error) error {
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		return fmt.Errorf("Güncelleme yedeği zaten mevcut veya erişilemiyor")
	}
	if err := os.Rename(target, backup); err != nil {
		return fmt.Errorf("Eski launcher taşınamadı: %w", err)
	}
	restore := func(cause error) error {
		if err := os.Rename(backup, target); err != nil {
			return fmt.Errorf("%v; yedek geri yüklenemedi: %w (yedek: %s)", cause, err, backup)
		}
		return cause
	}
	if err := os.Rename(staged, target); err != nil {
		return restore(err)
	}
	if err := start(); err != nil {
		if move := os.Rename(target, staged); move != nil {
			return fmt.Errorf("%v; yeni EXE taşınamadı: %w (yedek: %s)", err, move, backup)
		}
		return restore(err)
	}
	return nil
}
