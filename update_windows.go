//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Inspect only local processes; never stop them or persist their command lines.
func chaosProcesses(root string) (bool, bool, error) {
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[System.Text.Encoding]::UTF8; @(Get-CimInstance Win32_Process -Filter "Name='prismlauncher.exe' OR Name='java.exe' OR Name='javaw.exe'" | Select-Object Name,ExecutablePath,CommandLine,ProcessId,ParentProcessId) | ConvertTo-Json -Compress`)
	configureProcess(cmd)
	b, e := cmd.Output()
	if e != nil {
		return false, false, fmt.Errorf("Chaos oturum kontrolü başarısız: %w", e)
	}
	return classifyProcesses(root, b)
}
func gameClosed() error {
	prism, game, e := chaosProcesses(root)
	if e != nil {
		return e
	}
	if game {
		return fmt.Errorf("Mod dosyalarını değiştirmeden önce Chaos Minecraft oyununu kapatın.")
	}
	if prism {
		return fmt.Errorf("Mod dosyalarını değiştirmeden önce Chaos'un kendi Prism penceresini kapatın.")
	}
	return nil
}
func prepareLauncherUpdate(v Update) error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	dir := filepath.Dir(exe)
	tmp, e := os.MkdirTemp(dir, ".chaos-update-")
	if e != nil {
		return e
	}
	stage := filepath.Join(tmp, "new.exe")
	if e = fetchAsset(v.Repo, v.Tag, v.Asset, stage); e != nil {
		os.RemoveAll(tmp)
		return e
	}
	helper := filepath.Join(tmp, "helper.exe")
	if e = copyMissing(exe, helper); e != nil {
		os.RemoveAll(tmp)
		return e
	}
	cmd := exec.Command(helper, "--apply-chaos-update", exe, stage, strings.TrimPrefix(v.Asset.Digest, "sha256:"))
	configureProcess(cmd)
	if e = cmd.Start(); e != nil {
		os.RemoveAll(tmp)
		return e
	}
	go cmd.Wait()
	ui(func() { setBusy(false); u.NewProc("DestroyWindow").Call(hwnd) })
	return nil
}
func updateHelper() bool {
	if len(os.Args) != 5 || os.Args[1] != "--apply-chaos-update" {
		return false
	}
	target, stage, digest := os.Args[2], os.Args[3], os.Args[4]
	fail := func(e error) { msgBox.Call(0, ptr(e.Error()), ptr("Chaos güncelleme hatası"), 0x10) }
	self, e := os.Executable()
	if e != nil {
		fail(e)
		return true
	}
	if filepath.Dir(self) != filepath.Dir(stage) || filepath.Dir(filepath.Dir(stage)) != filepath.Dir(target) || filepath.Base(stage) != "new.exe" {
		fail(fmt.Errorf("Geçersiz güncelleme yolu"))
		return true
	}
	if e = verifySHA256(stage, digest); e != nil {
		fail(e)
		return true
	}
	backup := target + ".backup-" + fmt.Sprint(time.Now().UnixNano())
	for i := 0; i < 60; i++ {
		e = os.Rename(target, backup)
		if e == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if e != nil {
		fail(e)
		return true
	}
	if e = os.Rename(stage, target); e != nil {
		_ = os.Rename(backup, target)
		fail(e)
		return true
	}
	cmd := exec.Command(target)
	cmd.Dir = filepath.Dir(target)
	configureProcess(cmd)
	if e = cmd.Start(); e != nil {
		_ = os.Remove(target)
		_ = os.Rename(backup, target)
		fail(e)
	}
	return true
}

var _ = syscall.Handle(0)
