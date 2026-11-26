//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var u = syscall.NewLazyDLL("user32.dll")
var gd = syscall.NewLazyDLL("gdi32.dll")
var k = syscall.NewLazyDLL("kernel32.dll")
var sh = syscall.NewLazyDLL("shell32.dll")
var ole = syscall.NewLazyDLL("ole32.dll")

func proc(d *syscall.LazyDLL, n string) *syscall.LazyProc { return d.NewProc(n) }

var create = proc(u, "CreateWindowExW")
var send = proc(u, "SendMessageW")
var post = proc(u, "PostMessageW")
var setText = proc(u, "SetWindowTextW")
var getText = proc(u, "GetWindowTextW")
var getLen = proc(u, "GetWindowTextLengthW")
var def = proc(u, "DefWindowProcW")
var enable = proc(u, "EnableWindow")
var msgBox = proc(u, "MessageBoxW")
var brush = proc(gd, "CreateSolidBrush")
var font = proc(gd, "CreateFontW")
var setTC = proc(gd, "SetTextColor")
var setBC = proc(gd, "SetBkColor")
var setBM = proc(gd, "SetBkMode")

func ptr(s string) uintptr        { return uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(s))) }
func rgb(r, g, b uintptr) uintptr { return r | g<<8 | b<<16 }

var bg = rgb(14, 14, 21)
var panel = rgb(25, 25, 37)
var accent = rgb(179, 143, 255)
var white = rgb(241, 236, 250)
var muted = rgb(155, 153, 175)
var bgBrush, editBrush uintptr
var hwnd, statusH, accountH, codeH, creatorH, titleH, playH uintptr
var fields = map[int]uintptr{}
var controlColors = map[uintptr]uintptr{}
var btns []uintptr
var root string
var cfg Config
var acc Account
var busy bool
var verifyURL string
var publisherH, warningH uintptr
var warningSeen int
var publisherPending bool

//go:embed creator.bmp
var creatorBMP []byte

func drawCreator(dc uintptr) {
	if len(creatorBMP) < 55 {
		return
	}
	gd.NewProc("StretchDIBits").Call(dc, 250, 158, 128, 128, 0, 0, 128, 128, uintptr(unsafe.Pointer(&creatorBMP[54])), uintptr(unsafe.Pointer(&creatorBMP[14])), 0, 0x00CC0020)
}
func applyPublisher(v PublisherStatus) {
	switch v.State {
	case "verified":
		set(publisherH, "Yayıncı doğrulandı: @"+v.Username)
		if v.Warnings > warningSeen {
			warningSeen = v.Warnings
			set(warningH, fmt.Sprintf("Güvenlik uyarısı: %d tekrar bağlantı girişimi", v.Warnings))
			msgBox.Call(hwnd, ptr("TikTok hesabınız başka bir kuruluma bağlanmayı denedi. Bağlantı reddedildi."), ptr("Yayıncı güvenlik uyarısı"), 0x30)
		}
		publisherPending = false
	case "denied":
		set(publisherH, v.Message)
		if publisherPending {
			message(v.Message)
		}
		publisherPending = false
	case "pending":
		set(publisherH, "TikTok doğrulamasını tamamlayın.")
	default:
		if !publisherPending {
			set(publisherH, "Yayıncı hesabı bağlı değil.")
		}
	}
}
func startPublisher() {
	work(func() error {
		v, e := publisherRequest(root, "start")
		if e != nil {
			return e
		}
		if v.State == "pending" {
			u, e := url.Parse(v.URL)
			if e != nil || u.Scheme != "https" || u.Host != "www.tiktok.com" || u.Path != "/v2/auth/authorize/" {
				return fmt.Errorf("Geçersiz TikTok doğrulama adresi")
			}
			ui(func() { publisherPending = true; applyPublisher(v); openPath(v.URL) })
		} else {
			ui(func() { applyPublisher(v) })
		}
		return nil
	})
}
func pollPublisher() {
	if publisherEndpoint() == "" {
		return
	}
	for {
		v, e := publisherRequest(root, "status")
		if e == nil {
			ui(func() { applyPublisher(v) })
		}
		time.Sleep(10 * time.Second)
	}
}

var events = make(chan func(), 100)

const wmUpdate = 0x8001

func ui(f func())             { events <- f; post.Call(hwnd, wmUpdate, 0, 0) }
func set(h uintptr, s string) { setText.Call(h, ptr(s)) }
func text(h uintptr) string {
	n, _, _ := getLen.Call(h)
	b := make([]uint16, n+1)
	getText.Call(h, uintptr(unsafe.Pointer(&b[0])), n+1)
	return syscall.UTF16ToString(b)
}
func status(s string)      { ui(func() { set(statusH, s) }) }
func message(s string)     { msgBox.Call(hwnd, ptr(s), ptr("TikTok Chaos Launcher"), 0x40) }
func openPath(path string) { sh.NewProc("ShellExecuteW").Call(hwnd, ptr("open"), ptr(path), 0, 0, 1) }
func configureProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func mkFont(size int, weight int) uintptr {
	r, _, _ := font.Call(uintptr(int32(-size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, ptr("Segoe UI"))
	return r
}

var normal, small, large, medium, heroFont, tiny uintptr
var side = rgb(19, 19, 29)
var hero = rgb(32, 24, 48)
var sideBrush, heroBrush uintptr
var labelBrushes = map[uintptr]uintptr{}
var pages = map[int][]uintptr{}
var nav = map[int]uintptr{}
var buildPage, activePage int
var currentLabelBrush uintptr
var modsH uintptr
var buttonIDs = map[uintptr]int{}
var buttonFonts = map[uintptr]uintptr{}
var invalidate = proc(u, "InvalidateRect")
var showWindow = proc(u, "ShowWindow")

func page(n int, b uintptr) { buildPage = n; currentLabelBrush = b }
func switchPage(n int) {
	activePage = n
	for k, controls := range pages {
		for _, h := range controls {
			v := uintptr(0)
			if k == n {
				v = 5
			}
			showWindow.Call(h, v)
		}
	}
	if n == 2 {
		refreshMods()
	}
	invalidate.Call(hwnd, 0, 1)
	for _, h := range nav {
		invalidate.Call(h, 0, 1)
	}
}
func refreshMods() {
	if modsH == 0 {
		return
	}
	p := modsDir(root)
	entries, _ := os.ReadDir(p)
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".jar") {
			n++
		}
	}
	set(modsH, fmt.Sprintf("%d JAR dosyası", n))
}

type Rect struct{ L, T, R, B int32 }
type DrawItem struct {
	Type, ID, Item, Action, State uint32
	H, DC                         uintptr
	R                             Rect
	Data                          uintptr
}
type Paint struct {
	DC           uintptr
	Erase        int32
	R            Rect
	Restore, Inc int32
	Reserved     [32]byte
}

func fill(dc uintptr, r Rect, b uintptr) {
	u.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&r)), b)
}
func solid(dc uintptr, r Rect, c uintptr) {
	b, _, _ := brush.Call(c)
	fill(dc, r, b)
	gd.NewProc("DeleteObject").Call(b)
}
func round(dc uintptr, r Rect, c uintptr, rad int32) {
	b, _, _ := brush.Call(c)
	old, _, _ := gd.NewProc("SelectObject").Call(dc, b)
	np, _, _ := gd.NewProc("GetStockObject").Call(8)
	op, _, _ := gd.NewProc("SelectObject").Call(dc, np)
	gd.NewProc("RoundRect").Call(dc, uintptr(r.L), uintptr(r.T), uintptr(r.R), uintptr(r.B), uintptr(rad), uintptr(rad))
	gd.NewProc("SelectObject").Call(dc, op)
	gd.NewProc("SelectObject").Call(dc, old)
	gd.NewProc("DeleteObject").Call(b)
}
func drawButton(d *DrawItem) {
	id := buttonIDs[d.H]
	r := d.R
	background := bg
	if id >= 200 {
		background = side
	}
	if id == 1 {
		background = hero
	}
	if id == 3 || id == 7 || id == 8 || id == 9 || id == 2 || (id == 6 && activePage == 3) {
		background = panel
	}
	solid(d.DC, r, background)
	c := rgb(36, 36, 51)
	fg := white
	if id >= 200 {
		c = side
		fg = muted
		if id == 200+activePage {
			c = rgb(49, 37, 70)
			fg = accent
		}
	}
	if id == 1 || id == 3 || id == 6 {
		c = rgb(118, 69, 223)
		fg = rgb(255, 255, 255)
	}
	if d.State&1 != 0 {
		c = rgb(84, 51, 151)
	}
	if d.State&4 != 0 {
		c = rgb(41, 39, 52)
		fg = rgb(119, 115, 135)
	}
	round(d.DC, r, c, 12)
	if id >= 200 && id == 200+activePage {
		round(d.DC, Rect{0, 9, 4, r.B - 9}, accent, 4)
	}
	f := buttonFonts[d.H]
	if f == 0 {
		f = normal
	}
	old, _, _ := gd.NewProc("SelectObject").Call(d.DC, f)
	setBM.Call(d.DC, 1)
	setTC.Call(d.DC, fg)
	txt := text(d.H)
	flags := uintptr(0x25 | 0x800)
	if id >= 200 {
		r.L += 18
		flags = 0x24 | 0x800
	}
	u.NewProc("DrawTextW").Call(d.DC, ptr(txt), ^uintptr(0), uintptr(unsafe.Pointer(&r)), flags)
	gd.NewProc("SelectObject").Call(d.DC, old)
	if d.State&0x10 != 0 {
		q := d.R
		q.L += 4
		q.T += 4
		q.R -= 4
		q.B -= 4
		u.NewProc("DrawFocusRect").Call(d.DC, uintptr(unsafe.Pointer(&q)))
	}
}
func paintWindow(h uintptr) {
	var ps Paint
	dc, _, _ := u.NewProc("BeginPaint").Call(h, uintptr(unsafe.Pointer(&ps)))
	var r Rect
	u.NewProc("GetClientRect").Call(h, uintptr(unsafe.Pointer(&r)))
	fill(dc, r, bgBrush)
	fill(dc, Rect{0, 0, 200, r.B}, sideBrush)
	solid(dc, Rect{199, 0, 200, r.B}, rgb(42, 40, 53))
	solid(dc, Rect{224, 552, 976, 553}, rgb(43, 41, 55))
	switch activePage {
	case 0:
		round(dc, Rect{224, 92, 976, 356}, hero, 20)
		// Geometric violet motif, contained in the right-hand side of the hero.
		for i := int32(0); i < 5; i++ {
			x := int32(760) + i*33
			y := int32(126) + i*20
			round(dc, Rect{x, y, x + 68, y + 68}, rgb(uintptr(48+i*8), uintptr(31+i*3), uintptr(76+i*12)), 14)
		}
		round(dc, Rect{224, 376, 590, 530}, panel, 16)
		round(dc, Rect{608, 376, 976, 530}, panel, 16)
	case 1:
		round(dc, Rect{224, 92, 976, 530}, panel, 18)
	case 2:
		round(dc, Rect{224, 92, 976, 290}, panel, 18)
	case 3:
		round(dc, Rect{224, 92, 976, 530}, panel, 18)
	case 4:
		round(dc, Rect{224, 92, 976, 390}, panel, 18)
		drawCreator(dc)
	case 5, 6:
		round(dc, Rect{224, 92, 976, 530}, panel, 18)
	}
	u.NewProc("EndPaint").Call(h, uintptr(unsafe.Pointer(&ps)))
}

func control(class, s string, x, y, w, h, id int, style uintptr, f uintptr) uintptr {
	r, _, _ := create.Call(0, ptr(class), ptr(s), 0x50000000|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), hwnd, uintptr(id), 0, 0)
	send.Call(r, 0x30, f, 1)
	if buildPage >= 0 {
		pages[buildPage] = append(pages[buildPage], r)
	}
	return r
}
func label(s string, x, y, w, h int, f, col uintptr) uintptr {
	r := control("STATIC", s, x, y, w, h, 0, 0, f)
	controlColors[r] = col
	labelBrushes[r] = currentLabelBrush
	return r
}
func edit(id int, s string, x, y, w int) {
	fields[id] = control("EDIT", s, x, y, w, 28, id, 0x00800000|0x10000|0x80, normal)
}
func button(id int, s string, x, y, w, h int) uintptr {
	r := control("BUTTON", s, x, y, w, h, id, 0x10000|0xB, normal)
	buttonIDs[r] = id
	buttonFonts[r] = normal
	btns = append(btns, r)
	return r
}
func save() error {
	c := cfg
	c.ClientID = projectClientID()
	c.CreatorUser = creatorUser
	c.CreatorName = creatorName
	c.TikTokUser = ""
	c.Java = strings.TrimSpace(text(fields[105]))
	n, e := strconv.Atoi(text(fields[106]))
	if e != nil || n < 2 || n > 32 {
		return fmt.Errorf("RAM 2–32 GB arasında olmalı.")
	}
	c.RAM = n
	if e = configSave(root, c); e != nil {
		return e
	}
	if c.ClientID != cfg.ClientID {
		acc = Account{}
		set(accountH, "Minecraft hesabı: giriş yapılmadı")
	}
	cfg = c
	updateCreator()
	return nil
}
func updateCreator() {
	n := cfg.CreatorName
	if n == "" {
		n = "Geliştirici profili"
	}
	a := strings.TrimPrefix(cfg.CreatorUser, "@")
	if a == "" {
		a = "henüz ayarlanmadı"
	} else {
		a = "@" + a
	}
	set(creatorH, n+"  •  "+a)
}
func setBusy(b bool) {
	busy = b
	v := uintptr(1)
	if b {
		v = 0
	}
	for _, h := range btns {
		if buttonIDs[h] >= 200 {
			continue
		}
		enable.Call(h, v)
	}
	for _, h := range fields {
		enable.Call(h, v)
	}
	if b {
		if verifyBtn != 0 {
			enable.Call(verifyBtn, 1)
		}
		if copyBtn != 0 {
			enable.Call(copyBtn, 1)
		}
	}
}
func work(f func() error) {
	if busy {
		return
	}
	setBusy(true)
	go func() {
		e := f()
		ui(func() {
			setBusy(false)
			if activePage == 2 {
				refreshMods()
			}
			if e != nil {
				set(statusH, "Hata: "+e.Error())
				message(e.Error())
			}
		})
	}()
}

var verifyBtn, copyBtn uintptr
var loginCode string

func browseFolder() string {
	type BI struct {
		Owner, Root, Display, Title uintptr
		Flags                       uint32
		Callback, Param             uintptr
		Image                       int32
	}
	buf := make([]uint16, 260)
	b := BI{Owner: hwnd, Display: uintptr(unsafe.Pointer(&buf[0])), Title: ptr("Modların bulunduğu klasörü seçin"), Flags: 0x41}
	p, _, _ := sh.NewProc("SHBrowseForFolderW").Call(uintptr(unsafe.Pointer(&b)))
	if p == 0 {
		return ""
	}
	defer ole.NewProc("CoTaskMemFree").Call(p)
	out := make([]uint16, 32768)
	r, _, _ := sh.NewProc("SHGetPathFromIDListW").Call(p, uintptr(unsafe.Pointer(&out[0])))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(out)
}
func copyClipboard(s string) {
	if s == "" {
		return
	}
	r, _, _ := u.NewProc("OpenClipboard").Call(hwnd)
	if r == 0 {
		return
	}
	defer u.NewProc("CloseClipboard").Call()
	u.NewProc("EmptyClipboard").Call()
	b := syscall.StringToUTF16(s)
	m, _, _ := k.NewProc("GlobalAlloc").Call(2, uintptr(len(b)*2))
	if m == 0 {
		return
	}
	p, _, _ := k.NewProc("GlobalLock").Call(m)
	if p == 0 {
		k.NewProc("GlobalFree").Call(m)
		return
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(b)), b)
	k.NewProc("GlobalUnlock").Call(m)
	r, _, _ = u.NewProc("SetClipboardData").Call(13, m)
	if r == 0 {
		k.NewProc("GlobalFree").Call(m)
	}
}
func command(id int) {
	if id >= 200 && id <= 206 {
		switchPage(id - 200)
		return
	}
	if id == 11 {
		switchPage(3)
		return
	}
	if id == 8 {
		if verifyURL != "" {
			openPath(verifyURL)
		}
		return
	}
	if id == 9 {
		copyClipboard(loginCode)
		return
	}
	if busy {
		return
	}
	switch id {
	case 12:
		startPublisher()
	case 13:
		openPath("https://www.tiktok.com/@" + creatorUser)
	case 1:
		if e := save(); e != nil {
			message(e.Error())
			return
		}
		if e := checkChaos(modsDir(root)); e != nil {
			switchPage(2)
			message(e.Error())
			return
		}
		c := cfg
		work(func() error {
			return runPrism(root, c, true, status)
		})
	case 2, 3, 7:
		if e := save(); e != nil {
			message(e.Error())
			return
		}
		c := cfg
		work(func() error { return runPrism(root, c, false, status) })
	case 14:
		work(checkUpdatesUI)
	case 15:
		if pendingLauncher == nil {
			message("Önce güncellemeleri kontrol edin.")
			return
		}
		v := *pendingLauncher
		if !confirmUpdate(v, "Launcher") {
			return
		}
		work(func() error { status("Yeni launcher indiriliyor…"); return prepareLauncherUpdate(v) })
	case 16:
		if pendingMod == nil {
			message("Önce güncellemeleri kontrol edin.")
			return
		}
		v := *pendingMod
		if !confirmUpdate(v, "Chaos modu") {
			return
		}
		work(func() error {
			if e := gameClosed(); e != nil {
				return e
			}
			status("Yeni mod indiriliyor…")
			d, e := os.MkdirTemp(root, "mod-update-")
			if e != nil {
				return e
			}
			defer os.RemoveAll(d)
			p := filepath.Join(d, "new.jar")
			if e = fetchAsset(v.Repo, v.Tag, v.Asset, p); e != nil {
				return e
			}
			if e = gameClosed(); e != nil {
				return e
			}
			if e = replaceMod(root, p, v.Version); e != nil {
				return e
			}
			status("Mod güncellendi. Eski JAR yedeklendi.")
			return checkUpdatesUI()
		})
	case 4:
		p := browseFolder()
		if p == "" {
			return
		}
		work(func() error {
			if e := gameClosed(); e != nil {
				return e
			}
			n, e := copyMods(p, modsDir(root))
			if e == nil {
				status(fmt.Sprintf("%d mod JAR dosyası aktarıldı. Mevcut dosyalar korunur.", n))
				ui(refreshMods)
			}
			return e
		})
	case 5:
		p := modsDir(root)
		if e := mkdir(p); e != nil {
			message(e.Error())
			return
		}
		openPath(p)
	case 6:
		if e := save(); e != nil {
			message(e.Error())
			return
		}
		set(statusH, "Ayarlar kaydedildi.")
	case 10:
		openPath(root)
	}
}
func wnd(h uintptr, m uint32, w, l uintptr) uintptr {
	switch m {
	case 0x111:
		if w>>16 == 0 {
			command(int(w & 0xffff))
		}
		return 0
	case wmUpdate:
		select {
		case f := <-events:
			f()
		default:
		}
		return 0
	case 0xF:
		paintWindow(h)
		return 0
	case 0x2B:
		if l != 0 {
			drawButton((*DrawItem)(unsafe.Pointer(l)))
			return 1
		}
	case 0x138:
		setBM.Call(w, 2)
		col := controlColors[l]
		if col == 0 {
			col = white
		}
		setTC.Call(w, col)
		if b := labelBrushes[l]; b != 0 {
			c := bg
			switch b {
			case sideBrush:
				c = side
			case heroBrush:
				c = hero
			case editBrush:
				c = panel
			}
			setBC.Call(w, c)
			return b
		}
		setBC.Call(w, bg)
		return bgBrush
	case 0x133:
		setBC.Call(w, panel)
		setTC.Call(w, white)
		return editBrush
	case 0x10:
		if busy {
			r, _, _ := msgBox.Call(hwnd, ptr("İşlem devam ediyor. Kapatmak istediğinizden emin misiniz?"), ptr("Chaos Launcher"), 0x24)
			if r != 6 {
				return 0
			}
		}
		u.NewProc("DestroyWindow").Call(h)
		return 0
	case 2:
		u.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	r, _, _ := def.Call(h, uintptr(m), w, l)
	return r
}
func main() {
	runtime.LockOSThread()
	if updateHelper() {
		return
	}
	ole.NewProc("CoInitializeEx").Call(0, 2)
	defer ole.NewProc("CoUninitialize").Call()
	u.NewProc("SetProcessDPIAware").Call()
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	root = filepath.Join(base, "TikTokChaosLauncher")
	_ = mkdir(root)
	if e := initInstance(root); e != nil {
		message(e.Error())
		return
	}
	cfg = configLoad(root)
	cfg.ClientID = projectClientID()
	cfg.CreatorName = creatorName
	cfg.CreatorUser = creatorUser
	cfg.TikTokUser = ""
	bgBrush, _, _ = brush.Call(bg)
	editBrush, _, _ = brush.Call(panel)
	sideBrush, _, _ = brush.Call(side)
	heroBrush, _, _ = brush.Call(hero)
	normal = mkFont(16, 400)
	small = mkFont(14, 400)
	medium = mkFont(21, 600)
	large = mkFont(30, 700)
	heroFont = mkFont(44, 750)
	tiny = mkFont(12, 500)
	type WC struct {
		Size, Style                                             uint32
		WndProc                                                 uintptr
		ClsExtra, WndExtra                                      int32
		Instance, Icon, Cursor, Background, Menu, Class, IconSm uintptr
	}
	inst, _, _ := k.NewProc("GetModuleHandleW").Call(0)
	cursor, _, _ := u.NewProc("LoadCursorW").Call(0, 32512)
	class := ptr("ChaosLauncherNativeWindow")
	wc := WC{Size: uint32(unsafe.Sizeof(WC{})), WndProc: syscall.NewCallback(wnd), Instance: inst, Cursor: cursor, Background: bgBrush, Class: class}
	u.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	hwnd, _, _ = create.Call(0, class, ptr("TIKTOK CHAOS  |  Launcher"), 0x02CA0000, 0x80000000, 0x80000000, 1016, 690, 0, 0, inst, 0)
	if hwnd == 0 {
		return
	}
	finishUpdateCleanup()
	dark := int32(1)
	dwm := syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	if dwm.Find() == nil {
		dwm.Call(hwnd, 20, uintptr(unsafe.Pointer(&dark)), 4)
	}
	page(-1, sideBrush)
	label("CHAOS", 22, 23, 158, 33, large, white)
	label("L A U N C H E R", 24, 59, 158, 20, tiny, accent)
	label("KÜTÜPHANE", 24, 111, 158, 20, tiny, muted)
	for i, t := range []string{"Ana sayfa", "Hesabım", "Modlarım", "Ayarlar", "Geliştirici", "Yayıncı", "Güncellemeler"} {
		nav[i] = button(200+i, t, 14, 137+i*49, 172, 42)
	}
	label("TEST SÜRÜMÜ", 24, 572, 160, 19, tiny, muted)
	label("v"+launcherVersion+"  /  Windows", 24, 595, 170, 22, small, muted)
	page(-1, bgBrush)
	label("DURUM", 224, 569, 100, 20, tiny, accent)
	statusH = label("Hazır. Başlamak için hesabınızı bağlayın.", 224, 593, 752, 44, small, white)
	page(0, bgBrush)
	label("Ana sayfa", 224, 24, 490, 40, large, white)
	label("MINECRAFT  /  JAVA EDITION", 722, 37, 250, 22, tiny, muted)
	page(0, heroBrush)
	label("CANLI YAYIN MOD PAKETİ", 250, 115, 460, 22, tiny, accent)
	label("TIKTOK CHAOS", 246, 147, 495, 61, heroFont, white)
	label("Minecraft 1.21.11  •  Fabric 0.20.1", 250, 218, 490, 25, normal, muted)
	label("Yayınını aç. Kaos başlasın.", 250, 251, 490, 23, normal, white)
	playH = button(1, "▶   OYNA", 250, 290, 238, 46)
	buttonFonts[playH] = medium
	page(0, editBrush)
	currentLabelBrush = editBrush
	label("OYUN PROFİLİ", 244, 394, 326, 20, tiny, accent)
	label("Minecraft Java Edition", 244, 423, 325, 26, medium, white)
	label("Fabric  •  Java 21  •  Ayrı oyun klasörü", 244, 456, 326, 22, small, muted)
	label("Prism altyapısı otomatik indirilir.", 244, 487, 326, 22, small, white)
	label("YAYIN", 628, 394, 326, 20, tiny, accent)
	label("TikTok Chaos", 628, 423, 326, 26, medium, white)
	label("Yayın bağlantısı mod üzerinden çalışır.", 628, 456, 326, 22, small, muted)
	label("TikTok: yalnızca yayıncılar için", 628, 487, 326, 22, small, white)
	page(1, bgBrush)
	label("Hesabım", 224, 24, 740, 40, large, white)
	currentLabelBrush = editBrush
	label("MINECRAFT HESABI", 250, 120, 680, 22, tiny, accent)
	accountH = label("Microsoft hesabı • Prism üzerinden", 250, 155, 680, 36, medium, white)
	label("Minecraft Java lisansınızı Microsoft ile doğrulayın.", 250, 200, 680, 27, normal, muted)
	button(3, "Microsoft ile giriş", 250, 247, 240, 44)
	button(7, "Hesapları yönet", 505, 247, 200, 44)
	codeH = label("İlk girişte Prism açılır. Hesaplar > Microsoft hesabı ekle.", 250, 319, 680, 50, normal, white)
	label("Giriş bitince Chaos üzerinden OYNA’ya basın. Prism açık kalabilir.", 250, 380, 680, 45, normal, muted)
	label("Oyuncuların Client ID girmesine gerek yok.", 250, 438, 680, 26, small, muted)
	label("Şifre yalnızca Microsoft’un resmi giriş sayfasına yazılır.", 250, 471, 680, 26, small, muted)
	page(2, bgBrush)
	label("Modlarım", 224, 24, 740, 40, large, white)
	currentLabelBrush = editBrush
	label("TIKTOK CHAOS  /  MOD KÜTÜPHANESİ", 250, 118, 680, 23, tiny, accent)
	modsH = label("", 250, 152, 680, 35, medium, white)
	label("Mevcut CurseForge profilinden veya mods klasöründen aktar.", 250, 192, 680, 25, normal, muted)
	// The buttons are below the card, on the base background.
	page(2, bgBrush)
	button(4, "Modları içe aktar", 224, 310, 242, 46)
	button(5, "Mods klasörünü aç", 480, 310, 240, 46)
	button(10, "Oyun verileri", 734, 310, 242, 46)
	label("Chaos JAR pakete dahil değildir. Fabric API ve mod bağımlılıkları gereklidir.", 224, 388, 752, 48, normal, muted)
	page(3, bgBrush)
	label("Ayarlar", 224, 24, 740, 40, large, white)
	currentLabelBrush = editBrush
	label("OYUN", 250, 114, 680, 22, tiny, accent)
	label("Bellek (GB)", 250, 143, 320, 23, small, muted)
	edit(106, strconv.Itoa(cfg.RAM), 250, 168, 100)
	label("Java 21 yolu (boş: otomatik)", 390, 143, 550, 23, small, muted)
	edit(105, cfg.Java, 390, 168, 554)
	label("HESAP VE YAYIN", 250, 240, 680, 22, tiny, accent)
	label("Microsoft girişi: resmi Prism Portable altyapısı kullanılır.", 250, 280, 680, 25, normal, muted)
	label("TikTok bağlantısını Yayıncı bölümünden yönetebilirsiniz.", 250, 320, 680, 25, normal, muted)
	page(3, bgBrush)
	button(6, "Ayarları kaydet", 224, 474, 220, 42)
	button(2, "Prism kur / aç", 462, 474, 240, 42)
	page(4, bgBrush)
	label("Geliştirici", 224, 24, 740, 40, large, white)
	currentLabelBrush = editBrush
	label("TIKTOK CHAOS  /  PROJE SAHİBİ", 250, 120, 680, 22, tiny, accent)
	creatorH = label("", 400, 162, 530, 40, medium, white)
	updateCreator()
	label("TikTok Chaos • Proje geliştiricisi", 400, 215, 530, 30, normal, muted)
	label("Profil bilgileri sabittir.", 400, 259, 530, 25, small, muted)
	page(4, bgBrush)
	button(13, "TikTok profilini aç", 224, 410, 250, 42)
	label("© "+creatorName+" • TikTok Chaos", 224, 484, 752, 24, small, muted)
	page(5, bgBrush)
	label("Yayıncı", 224, 24, 740, 40, large, white)
	currentLabelBrush = editBrush
	label("TIKTOK • TEK HESAP BAĞLANTISI", 250, 120, 680, 22, tiny, accent)
	publisherH = label("Yayıncı hesabı bağlı değil.", 250, 155, 690, 50, medium, white)
	if publisherEndpoint() == "" {
		set(publisherH, "Doğrulama sunucusu henüz etkin değil.")
	}
	label("Normal oyuncuların TikTok doğrulaması yapması gerekmez.", 250, 220, 690, 27, normal, muted)
	button(12, "TikTok ile yayıncı doğrula", 250, 268, 320, 44)
	warningH = label("Güvenlik uyarısı yok.", 250, 340, 690, 45, normal, accent)
	label("Bir TikTok hesabı yalnızca bir kuruluma bağlanabilir.", 250, 410, 680, 24, small, muted)
	label("Tekrar bağlantı denemeleri ilk yayıncıya bildirilir.", 250, 444, 680, 24, small, muted)
	page(6, bgBrush)
	label("Güncellemeler", 224, 24, 740, 40, large, white)
	currentLabelBrush = editBrush
	label("LAUNCHER VE MOD SÜRÜMLERİ", 250, 120, 680, 22, tiny, accent)
	updateH = label("Launcher v"+launcherVersion+" • Henüz kontrol edilmedi", 250, 155, 680, 50, medium, white)
	modUpdateH = label("Chaos modu • Henüz kontrol edilmedi", 250, 220, 680, 45, normal, white)
	button(14, "Güncellemeleri kontrol et", 250, 285, 300, 44)
	button(15, "Launcher güncelle", 250, 342, 240, 44)
	button(16, "Modu güncelle", 505, 342, 240, 44)
	label("GitHub: y0zssh111/Chaos-Launcher • Açılışta kontrol edilir.", 250, 410, 680, 25, small, muted)
	button(10, "Veri klasörünü aç", 250, 455, 240, 42)
	switchPage(0)
	u.NewProc("ShowWindow").Call(hwnd, 5)
	u.NewProc("UpdateWindow").Call(hwnd)
	go pollPublisher()
	if releaseRepo(root) != "" {
		work(checkUpdatesUI)
	}
	type MSG struct {
		H       uintptr
		M       uint32
		W, L    uintptr
		Time    uint32
		X, Y    int32
		Private uint32
	}
	var msg MSG
	for {
		r, _, _ := u.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		u.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		u.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}

var updateH, modUpdateH uintptr
var pendingLauncher, pendingMod *Update

func checkUpdatesUI() error {
	status("Güncellemeler kontrol ediliyor…")
	l, m, e := availableUpdates(root)
	if e != nil {
		ui(func() {
			pendingLauncher = nil
			pendingMod = nil
			set(updateH, "Güncelleme kaynağı kullanılamıyor.")
			set(modUpdateH, e.Error())
		})
		return e
	}
	ui(func() {
		pendingLauncher = l
		pendingMod = m
		ls := "Launcher v" + launcherVersion + " • güncel"
		ms := "Chaos modu • yeni sürüm yok"
		if l != nil {
			ls = "Yeni launcher: v" + l.Version
		}
		if m != nil {
			ms = "Yeni Chaos modu: v" + m.Version
		}
		set(updateH, ls)
		set(modUpdateH, ms)
		set(statusH, "Güncelleme kontrolü tamamlandı. "+ls)
	})
	return nil
}
func confirmUpdate(v Update, kind string) bool {
	r, _, _ := msgBox.Call(hwnd, ptr(kind+" v"+v.Version+"\nKaynak: github.com/"+v.Repo+"\nBu depoya güveniyorsanız güncellemeyi yükleyin."), ptr("Güncelleme onayı"), 0x24)
	return r == 6
}
