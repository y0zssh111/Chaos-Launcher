//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	if name, err := syscall.UTF16PtrFromString(tmp); err == nil {
		_ = syscall.SetFileAttributes(name, syscall.FILE_ATTRIBUTE_HIDDEN)
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
	cmd := exec.Command(helper, "--apply-chaos-update", exe, stage, strings.TrimPrefix(v.Asset.Digest, "sha256:"), strconv.Itoa(os.Getpid()))
	configureProcess(cmd)
	if e = cmd.Start(); e != nil {
		os.RemoveAll(tmp)
		return e
	}
	go cmd.Wait()
	ui(func() { setBusy(false); u.NewProc("DestroyWindow").Call(hwnd) })
	return nil
}

// Wait for the original launcher to exit before touching its executable.
func waitForLauncher(pid uint32) error {
	h, err := syscall.OpenProcess(0x00100000, false, pid) // SYNCHRONIZE
	if err == syscall.Errno(87) {
		return nil
	} // already exited
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	result, err := syscall.WaitForSingleObject(h, 60000)
	if err != nil {
		return err
	}
	if result != 0 {
		return fmt.Errorf("Eski launcher zamanında kapanmadı (kod %d)", result)
	}
	return nil
}
func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }
func validUpdateDir(target, dir string) bool {
	return samePath(filepath.Dir(target), filepath.Dir(dir)) && strings.HasPrefix(filepath.Base(dir), ".chaos-update-")
}
func updateHelper() bool {
	if len(os.Args) != 6 || os.Args[1] != "--apply-chaos-update" {
		return false
	}
	target, stage, digest := os.Args[2], os.Args[3], os.Args[4]
	fail := func(e error) { msgBox.Call(0, ptr(e.Error()), ptr("Chaos güncelleme hatası"), 0x10) }
	self, e := os.Executable()
	if e != nil {
		fail(e)
		return true
	}
	dir := filepath.Dir(stage)
	if !samePath(filepath.Dir(self), dir) || !validUpdateDir(target, dir) || filepath.Base(self) != "helper.exe" || filepath.Base(stage) != "new.exe" {
		fail(fmt.Errorf("Geçersiz güncelleme yolu"))
		return true
	}
	if e = verifySHA256(stage, digest); e != nil {
		fail(e)
		return true
	}
	pid, e := strconv.ParseUint(os.Args[5], 10, 32)
	if e != nil || pid == 0 {
		fail(fmt.Errorf("Geçersiz launcher işlem kimliği"))
		return true
	}
	if e = waitForLauncher(uint32(pid)); e != nil {
		fail(e)
		return true
	}
	backup := filepath.Join(dir, "previous.exe")
	e = swapExecutable(target, stage, backup, func() error {
		// Normal GUI launch, never configureProcess: that hides the startup window.
		cmd := exec.Command(target, "--finish-chaos-update", dir)
		cmd.Dir = filepath.Dir(target)
		return cmd.Start()
	})
	if e != nil {
		fail(e)
	}
	return true
}

// Remove only known files in the specific update directory, after helper exits.
func finishUpdateCleanup() {
	if len(os.Args) != 3 || os.Args[1] != "--finish-chaos-update" {
		return
	}
	self, e := os.Executable()
	if e != nil {
		return
	}
	dir := os.Args[2]
	if !validUpdateDir(self, dir) {
		return
	}
	go func() {
		for i := 0; i < 120; i++ {
			_ = os.Remove(filepath.Join(dir, "helper.exe"))
			_ = os.Remove(filepath.Join(dir, "new.exe"))
			_ = os.Remove(filepath.Join(dir, "previous.exe"))
			if e := os.Remove(dir); e == nil || os.IsNotExist(e) {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
}
