//go:build windows

package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Only console helpers use CREATE_NO_WINDOW. Prism is a visible GUI.
func configurePrismProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{} }

func windowPID(h uintptr) uint32 {
	var pid uint32
	u.NewProc("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}
func processImage(pid uint32) string {
	h, e := syscall.OpenProcess(0x1000, false, pid)
	if e != nil {
		return ""
	}
	defer syscall.CloseHandle(h)
	var b [32768]uint16
	n := uint32(len(b))
	r, _, _ := k.NewProc("QueryFullProcessImageNameW").Call(uintptr(h), 0, uintptr(unsafe.Pointer(&b[0])), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(b[:n])
}
func windowTitle(h uintptr) string {
	var b [512]uint16
	getText.Call(h, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
	return syscall.UTF16ToString(b[:])
}

// Use a single callback for all enumerations; no accumulating Go callback slots.
var enumAction func(uintptr) bool
var enumCallback = syscall.NewCallback(func(h, l uintptr) uintptr {
	if enumAction != nil && !enumAction(h) {
		return 0
	}
	return 1
})

// Called only on the UI thread.
func enumWindows(f func(uintptr) bool) {
	enumAction = f
	u.NewProc("EnumWindows").Call(enumCallback, 0)
	enumAction = nil
}
func showPrismAccountWindow(root string) {
	ui(func() {
		prefix := strings.TrimRight(normWin(filepath.Join(root, "prism-runtime")), `\`) + `\`
		enumWindows(func(h uintptr) bool {
			title := windowTitle(h)
			if !strings.Contains(strings.ToLower(title), "prism launcher") {
				return true
			}
			path := processImage(windowPID(h))
			if !strings.HasPrefix(normWin(path), prefix) || !strings.EqualFold(filepath.Base(path), "prismlauncher.exe") {
				return true
			}
			showWindow.Call(h, 9)
			u.NewProc("SetForegroundWindow").Call(h)
			return false
		})
	})
}

const wmTray = 0x8002
const trayOpen = 1001
const trayExit = 1002

var trayTaskbarMessage uint32
var trayIcon uintptr
var ownTrayIcon, trayWanted, watchingGame bool
var watchGeneration uint64

type notifyIconData struct {
	Size                uint32
	H                   uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                [16]byte
	BalloonIcon         uintptr
}

func makeTrayIcon() uintptr {
	// 32x32 monochrome-alpha mask and a purple tile with a white C.
	const n = 32
	and := make([]byte, n*n/8)
	xor := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			i := (y*n + x) * 4
			dx, dy := x-15, y-15
			if dx*dx+dy*dy > 15*15 {
				and[y*4+x/8] |= 1 << uint(7-x%8)
				continue
			}
			xor[i], xor[i+1], xor[i+2], xor[i+3] = 215, 69, 118, 255
			if (x >= 7 && x <= 24 && ((y >= 7 && y <= 11) || (y >= 21 && y <= 25))) || (x >= 7 && x <= 11 && y >= 11 && y <= 21) {
				xor[i], xor[i+1], xor[i+2] = 255, 255, 255
			}
		}
	}
	inst, _, _ := k.NewProc("GetModuleHandleW").Call(0)
	h, _, _ := u.NewProc("CreateIcon").Call(inst, n, n, 1, 32, uintptr(unsafe.Pointer(&and[0])), uintptr(unsafe.Pointer(&xor[0])))
	if h != 0 {
		ownTrayIcon = true
		return h
	}
	h, _, _ = u.NewProc("LoadIconW").Call(0, 32512)
	return h
}
func trayData() notifyIconData {
	d := notifyIconData{H: hwnd, ID: 1, Flags: 7, Callback: wmTray, Icon: trayIcon}
	d.Size = uint32(unsafe.Sizeof(d))
	copy(d.Tip[:], syscall.StringToUTF16("Chaos Launcher v"+launcherVersion))
	return d
}
func initTray() {
	trayIcon = makeTrayIcon()
	m, _, _ := u.NewProc("RegisterWindowMessageW").Call(ptr("TaskbarCreated"))
	trayTaskbarMessage = uint32(m)
}
func addTray() bool {
	if trayIcon == 0 {
		return false
	}
	d := trayData()
	r, _, _ := sh.NewProc("Shell_NotifyIconW").Call(0, uintptr(unsafe.Pointer(&d)))
	if r == 0 {
		return false
	}
	trayWanted = true
	return true
}
func removeTray() {
	if trayWanted {
		d := trayData()
		sh.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&d)))
		trayWanted = false
	}
	if ownTrayIcon && trayIcon != 0 {
		u.NewProc("DestroyIcon").Call(trayIcon)
		trayIcon = 0
	}
}
func hideToTray() bool {
	if !trayWanted && !addTray() {
		return false
	}
	showWindow.Call(hwnd, 0)
	return true
}
func restoreLauncher() {
	watchGeneration++
	watchingGame = false
	showWindow.Call(hwnd, 9)
	u.NewProc("SetForegroundWindow").Call(hwnd)
}
func handleTray(l uintptr) {
	switch uint32(l) & 0xffff {
	case 0x202, 0x203:
		restoreLauncher()
	case 0x205, 0x7b:
		trayMenu()
	}
}
func trayMenu() {
	menu, _, _ := u.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer u.NewProc("DestroyMenu").Call(menu)
	u.NewProc("AppendMenuW").Call(menu, 0, trayOpen, ptr("Chaos Launcher'ı aç"))
	u.NewProc("AppendMenuW").Call(menu, 0x800, 0, 0)
	u.NewProc("AppendMenuW").Call(menu, 0, trayExit, ptr("Çıkış"))
	var pt struct{ X, Y int32 }
	u.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pt)))
	u.NewProc("SetForegroundWindow").Call(hwnd)
	id, _, _ := u.NewProc("TrackPopupMenu").Call(menu, 0x102, uintptr(pt.X), uintptr(pt.Y), 0, hwnd, 0)
	post.Call(hwnd, 0, 0, 0)
	switch id {
	case trayOpen:
		restoreLauncher()
	case trayExit:
		if busy {
			message("İşlem sürüyor; tamamlandıktan sonra çıkın.")
			return
		}
		u.NewProc("DestroyWindow").Call(hwnd)
	}
}
func glfwWindows() map[int]bool {
	ids := map[int]bool{}
	enumWindows(func(h uintptr) bool {
		v, _, _ := u.NewProc("IsWindowVisible").Call(h)
		if v == 0 {
			return true
		}
		var b [128]uint16
		u.NewProc("GetClassNameW").Call(h, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
		if strings.EqualFold(syscall.UTF16ToString(b[:]), "GLFW") {
			ids[int(windowPID(h))] = true
		}
		return true
	})
	return ids
}
func beginGameWatch() {
	if watchingGame {
		return
	}
	watchingGame = true
	watchGeneration++
	gen := watchGeneration
	// Bounded, low-frequency polling only after a play request. No continuous scans while idle.
	go func() {
		defer ui(func() {
			if watchGeneration == gen {
				watchingGame = false
			}
		})
		for i := 0; i < 180; i++ {
			time.Sleep(5 * time.Second)
			ch := make(chan map[int]bool, 1)
			ui(func() {
				if watchGeneration != gen {
					ch <- nil
					return
				}
				ch <- glfwWindows()
			})
			ids := <-ch
			if ids == nil {
				return
			}
			if len(ids) == 0 {
				continue
			}
			data, e := scanChaosProcesses()
			if e != nil || !ownsGameWindow(root, data, ids) {
				continue
			}
			ui(func() {
				if watchGeneration == gen {
					set(statusH, "Minecraft açıldı. Launcher bildirim alanında çalışıyor.")
					hideToTray()
				}
			})
			return
		}
	}()
}
