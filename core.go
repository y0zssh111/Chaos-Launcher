package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const mcVersion = "1.21.11"
const fabricVersion = "0.20.1"
const launcherVersion = "0.4.4"

var client = &http.Client{Timeout: 90 * time.Second}

type Config struct {
	ClientID, CreatorUser, CreatorName, TikTokUser, Java string
	RAM                                                  int
}
type Account struct {
	Name, ID, Token, XUID string
	Expires               time.Time
}
type Download struct {
	URL, SHA1, Path string
	Size            int64
}
type Rule struct {
	Action   string
	OS       struct{ Name, Arch, Version string }
	Features map[string]bool
}
type Library struct {
	Name, URL string
	Rules     []Rule
	Downloads struct {
		Artifact    Download
		Classifiers map[string]Download
	}
	Natives map[string]string
	Extract struct{ Exclude []string }
}
type Version struct {
	ID, MainClass, Type, Assets, InheritsFrom string
	Downloads                                 map[string]Download
	AssetIndex                                Download
	AssetIndexID                              string `json:"-"`
	Libraries                                 []Library
	Arguments                                 struct{ Game, JVM []json.RawMessage }
	JavaVersion                               struct{ MajorVersion int }
}
type Manifest struct {
	Versions []struct{ ID, URL, SHA1 string }
}
type AssetIndex struct {
	Objects map[string]struct {
		Hash string
		Size int64
	}
}
type Game struct {
	Root, Java, Main   string
	JVM, Args          []json.RawMessage
	Classpath          []string
	AssetID, NativeDir string
	Version            string
}

func mkdir(p string) error { return os.MkdirAll(p, 0755) }
func getJSON(u string, v any) error {
	r, e := client.Get(u)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return fmt.Errorf("HTTP %d: %s", r.StatusCode, u)
	}
	return json.NewDecoder(io.LimitReader(r.Body, 32<<20)).Decode(v)
}
func requestJSON(u string, payload any, out any) error {
	b, _ := json.Marshal(payload)
	r, e := client.Post(u, "application/json", bytes.NewReader(b))
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return fmt.Errorf("Kimlik doğrulama HTTP %d (%s)", r.StatusCode, u)
	}
	return json.NewDecoder(r.Body).Decode(out)
}
func form(u string, v url.Values, out any) error {
	r, e := client.PostForm(u, v)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}
func login(id string, show func(string, string), status func(string)) (Account, error) {
	var a Account
	if !regexp.MustCompile(`^[a-fA-F0-9-]{36}$`).MatchString(id) {
		return a, errors.New("Onaylı Microsoft uygulama Client ID gerekli (36 karakter).")
	}
	const base = "https://login.microsoftonline.com/consumers/oauth2/v2.0/"
	var d struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
		URI        string `json:"verification_uri"`
		Interval   int    `json:"interval"`
		Expires    int    `json:"expires_in"`
		Error      string `json:"error_description"`
	}
	if e := form(base+"devicecode", url.Values{"client_id": {id}, "scope": {"XboxLive.signin offline_access"}}, &d); e != nil {
		return a, e
	}
	if d.DeviceCode == "" {
		return a, fmt.Errorf("Microsoft: %s", d.Error)
	}
	show(d.UserCode, d.URI)
	if d.Interval < 5 {
		d.Interval = 5
	}
	end := time.Now().Add(time.Duration(d.Expires) * time.Second)
	var t struct {
		Access      string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	for time.Now().Before(end) {
		time.Sleep(time.Duration(d.Interval) * time.Second)
		t.Access = ""
		t.Error = ""
		if e := form(base+"token", url.Values{"client_id": {id}, "grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {d.DeviceCode}}, &t); e != nil {
			return a, e
		}
		if t.Access != "" {
			break
		}
		switch t.Error {
		case "authorization_pending":
		case "slow_down":
			d.Interval += 5
		default:
			return a, fmt.Errorf("Microsoft: %s", t.Description)
		}
	}
	if t.Access == "" {
		return a, errors.New("Giriş kodunun süresi doldu.")
	}
	status("Xbox ve Minecraft hesabı doğrulanıyor…")
	type Xbox struct {
		Token         string
		DisplayClaims struct {
			XUI []struct {
				UHS string
				XID string
			}
		}
	}
	var x, y Xbox
	if e := requestJSON("https://user.auth.xboxlive.com/user/authenticate", map[string]any{"Properties": map[string]any{"AuthMethod": "RPS", "SiteName": "user.auth.xboxlive.com", "RpsTicket": "d=" + t.Access}, "RelyingParty": "http://auth.xboxlive.com", "TokenType": "JWT"}, &x); e != nil {
		return a, e
	}
	if x.Token == "" {
		return a, errors.New("Xbox token alınamadı")
	}
	if e := requestJSON("https://xsts.auth.xboxlive.com/xsts/authorize", map[string]any{"Properties": map[string]any{"SandboxId": "RETAIL", "UserTokens": []string{x.Token}}, "RelyingParty": "rp://api.minecraftservices.com/", "TokenType": "JWT"}, &y); e != nil {
		return a, e
	}
	if y.Token == "" || len(y.DisplayClaims.XUI) == 0 {
		return a, errors.New("Xbox XSTS hesabı doğrulanamadı")
	}
	var m struct {
		Access  string `json:"access_token"`
		Expires int    `json:"expires_in"`
	}
	if e := requestJSON("https://api.minecraftservices.com/authentication/login_with_xbox", map[string]string{"identityToken": "XBL3.0 x=" + y.DisplayClaims.XUI[0].UHS + ";" + y.Token}, &m); e != nil {
		return a, e
	}
	if m.Access == "" {
		return a, errors.New("Minecraft erişim tokenı alınamadı")
	}
	req, _ := http.NewRequest("GET", "https://api.minecraftservices.com/minecraft/profile", nil)
	req.Header.Set("Authorization", "Bearer "+m.Access)
	r, e := client.Do(req)
	if e != nil {
		return a, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return a, fmt.Errorf("Minecraft Java profili bulunamadı (HTTP %d). Lisansınızı kontrol edin.", r.StatusCode)
	}
	if e = json.NewDecoder(r.Body).Decode(&a); e != nil {
		return a, e
	}
	if a.ID == "" || a.Name == "" {
		return a, errors.New("Minecraft Java profili eksik")
	}
	a.Token = m.Access
	a.Expires = time.Now().Add(time.Duration(m.Expires) * time.Second)
	a.XUID = y.DisplayClaims.XUI[0].XID
	return a, nil
}
func verify(path, hash string, size int64) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	s, e := f.Stat()
	if e != nil || s.IsDir() || s.Size() == 0 || (size > 0 && s.Size() != size) {
		return false
	}
	if hash == "" {
		return true
	}
	h := sha1.New()
	_, e = io.Copy(h, f)
	return e == nil && strings.EqualFold(hex.EncodeToString(h.Sum(nil)), hash)
}
func download(d Download, p string) error {
	if verify(p, d.SHA1, d.Size) {
		return nil
	}
	if !strings.HasPrefix(d.URL, "https://") {
		return errors.New("Güvensiz indirme URL'si")
	}
	if e := mkdir(filepath.Dir(p)); e != nil {
		return e
	}
	var last error
	for i := 0; i < 3; i++ {
		last = func() error {
			r, e := client.Get(d.URL)
			if e != nil {
				return e
			}
			defer r.Body.Close()
			if r.StatusCode != 200 {
				return fmt.Errorf("HTTP %d: %s", r.StatusCode, d.URL)
			}
			f, e := os.CreateTemp(filepath.Dir(p), ".download-*")
			if e != nil {
				return e
			}
			tmp := f.Name()
			defer os.Remove(tmp)
			_, e = io.Copy(f, r.Body)
			ce := f.Close()
			if e != nil {
				return e
			}
			if ce != nil {
				return ce
			}
			if !verify(tmp, d.SHA1, d.Size) {
				return fmt.Errorf("İndirme doğrulanamadı: %s", filepath.Base(p))
			}
			_ = os.Remove(p)
			return os.Rename(tmp, p)
		}()
		if last == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return last
}
func allowed(rules []Rule) bool {
	if len(rules) == 0 {
		return true
	}
	ok := false
	for _, r := range rules {
		if r.OS.Name != "" && r.OS.Name != "windows" {
			continue
		}
		if r.OS.Arch != "" && r.OS.Arch != "x86_64" && r.OS.Arch != "amd64" {
			continue
		}
		if r.OS.Version != "" {
			matched, e := regexp.MatchString(r.OS.Version, "10.0")
			if e != nil || !matched {
				continue
			}
		}
		match := true
		for _, v := range r.Features {
			if v {
				match = false
			}
		}
		if match {
			ok = r.Action == "allow"
		}
	}
	return ok
}
func mavenPath(s string) (string, error) {
	p := strings.Split(s, ":")
	if len(p) < 3 || len(p) > 4 {
		return "", fmt.Errorf("Maven adı geçersiz: %s", s)
	}
	ext := "jar"
	last := strings.Split(p[len(p)-1], "@")
	p[len(p)-1] = last[0]
	if len(last) > 1 {
		ext = last[1]
	}
	base := p[1] + "-" + p[2]
	if len(p) == 4 {
		base += "-" + p[3]
	}
	return strings.ReplaceAll(p[0], ".", "/") + "/" + p[1] + "/" + p[2] + "/" + base + "." + ext, nil
}
func unzip(path, dest string, exclude []string) error {
	r, e := zip.OpenReader(path)
	if e != nil {
		return e
	}
	defer r.Close()
	for _, z := range r.File {
		name := z.Name
		skip := false
		for _, x := range exclude {
			if strings.HasPrefix(name, x) {
				skip = true
			}
		}
		if skip || z.FileInfo().IsDir() {
			continue
		}
		if z.Mode()&os.ModeSymlink != 0 {
			return errors.New("ZIP sembolik bağlantı içeriyor")
		}
		p := filepath.Join(dest, filepath.FromSlash(name))
		rel, e := filepath.Rel(dest, p)
		if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return errors.New("Geçersiz ZIP yolu")
		}
		if e = mkdir(filepath.Dir(p)); e != nil {
			return e
		}
		src, e := z.Open()
		if e != nil {
			return e
		}
		f, e := os.Create(p)
		if e != nil {
			src.Close()
			return e
		}
		_, e = io.Copy(f, src)
		src.Close()
		f.Close()
		if e != nil {
			return e
		}
	}
	return nil
}
func javaOK(p string) bool {
	if p == "" {
		return false
	}
	cmd := exec.Command(p, "-version")
	configureProcess(cmd)
	b, e := cmd.CombinedOutput()
	return e == nil && (strings.Contains(string(b), `version "21.`) || strings.Contains(string(b), `version "21"`))
}
func findJava(root, custom string, status func(string)) (string, error) {
	if custom != "" {
		if javaOK(custom) {
			return custom, nil
		}
		return "", errors.New("Seçilen Java, Java 21 değil veya çalıştırılamıyor.")
	}
	var candidates []string
	if p, e := exec.LookPath("java.exe"); e == nil {
		candidates = append(candidates, p)
	}
	if h := os.Getenv("JAVA_HOME"); h != "" {
		candidates = append(candidates, filepath.Join(h, "bin", "java.exe"))
	}
	for _, pat := range []string{filepath.Join(root, "runtime", "*", "bin", "java.exe"), filepath.Join(os.Getenv("ProgramFiles"), "Eclipse Adoptium", "*", "bin", "java.exe"), filepath.Join(os.Getenv("APPDATA"), ".minecraft", "runtime", "*", "windows-x64", "*", "bin", "java.exe"), filepath.Join(os.Getenv("USERPROFILE"), "curseforge", "minecraft", "Install", "runtime", "*", "windows-x64", "*", "bin", "java.exe")} {
		m, _ := filepath.Glob(pat)
		candidates = append(candidates, m...)
	}
	for _, p := range candidates {
		if javaOK(p) {
			return p, nil
		}
	}
	status("Java 21 indiriliyor (Eclipse Adoptium)…")
	var rel []struct {
		Binary struct {
			Package struct {
				Link, Checksum string
				Size           int64
			}
		}
	}
	if e := getJSON("https://api.adoptium.net/v3/assets/latest/21/hotspot?architecture=x64&image_type=jre&os=windows&vendor=eclipse", &rel); e != nil {
		return "", e
	}
	if len(rel) == 0 || rel[0].Binary.Package.Link == "" {
		return "", errors.New("Java 21 paketi bulunamadı")
	}
	d := rel[0].Binary.Package
	p := filepath.Join(root, "runtime.zip")
	if e := download(Download{URL: d.Link, Size: d.Size}, p); e != nil {
		return "", e
	}
	if e := verifySHA256(p, d.Checksum); e != nil {
		os.Remove(p)
		return "", e
	}
	if e := unzip(p, filepath.Join(root, "runtime"), nil); e != nil {
		return "", e
	}
	os.Remove(p)
	matches, _ := filepath.Glob(filepath.Join(root, "runtime", "*", "bin", "java.exe"))
	for _, p := range matches {
		if javaOK(p) {
			return p, nil
		}
	}
	return "", errors.New("Java 21 kurulamadı")
}
func verifySHA256(p, want string) error {
	if want == "" {
		return errors.New("Java checksum eksik")
	}
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	h := sha256.New()
	if _, e = io.Copy(h, f); e != nil {
		return e
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want) {
		return errors.New("Java SHA256 doğrulanamadı")
	}
	return nil
}
func readVersion(path string) (Version, error) {
	var v Version
	b, e := os.ReadFile(path)
	if e != nil {
		return v, e
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return v, e
	}
	var extra struct{ AssetIndex struct{ ID string } }
	_ = json.Unmarshal(b, &extra)
	v.AssetIndexID = extra.AssetIndex.ID
	return v, nil
}
func install(root, java string, status func(string)) (Game, error) {
	g := Game{Root: root, Version: "fabric-" + fabricVersion + "-" + mcVersion}
	if e := mkdir(root); e != nil {
		return g, e
	}
	for _, p := range []string{"mods", "assets", "versions", "libraries", "logs", "natives"} {
		if e := mkdir(filepath.Join(root, p)); e != nil {
			return g, e
		}
	}
	status("Minecraft ve Fabric sürüm bilgisi alınıyor…")
	var manifest Manifest
	if e := getJSON("https://piston-meta.mojang.com/mc/game/version_manifest_v2.json", &manifest); e != nil {
		return g, e
	}
	var d Download
	for _, v := range manifest.Versions {
		if v.ID == mcVersion {
			d = Download{URL: v.URL, SHA1: v.SHA1}
			break
		}
	}
	if d.URL == "" {
		return g, errors.New("Minecraft sürümü bulunamadı")
	}
	vp := filepath.Join(root, "versions", mcVersion, mcVersion+".json")
	if e := download(d, vp); e != nil {
		return g, e
	}
	v, e := readVersion(vp)
	if e != nil {
		return g, e
	}
	fp := filepath.Join(root, "versions", g.Version, g.Version+".json")
	if e = download(Download{URL: "https://meta.fabricmc.net/v2/versions/loader/" + mcVersion + "/" + fabricVersion + "/profile/json"}, fp); e != nil {
		return g, e
	}
	f, e := readVersion(fp)
	if e != nil {
		return g, e
	}
	if f.MainClass == "" || v.Downloads["client"].URL == "" || v.AssetIndexID == "" {
		return g, errors.New("Sürüm verisi eksik")
	}
	g.Main = f.MainClass
	g.JVM = append(v.Arguments.JVM, f.Arguments.JVM...)
	g.Args = append(v.Arguments.Game, f.Arguments.Game...)
	g.AssetID = v.AssetIndexID
	g.NativeDir = filepath.Join(root, "natives")
	status("Minecraft kütüphaneleri indiriliyor…")
	libs := append(f.Libraries, v.Libraries...)
	seen := map[string]bool{}
	for i, l := range libs {
		if !allowed(l.Rules) {
			continue
		}
		p, e := mavenPath(l.Name)
		if e != nil {
			return g, e
		}
		a := l.Downloads.Artifact
		if a.Path != "" {
			p = a.Path
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		if a.URL == "" {
			base := l.URL
			if base == "" {
				base = "https://libraries.minecraft.net/"
			}
			a.URL = strings.TrimRight(base, "/") + "/" + p
		}
		dest := filepath.Join(root, "libraries", filepath.FromSlash(p))
		if e = download(a, dest); e != nil {
			return g, e
		}
		g.Classpath = append(g.Classpath, dest)
		if strings.Contains(filepath.Base(p), "natives-windows") {
			if e = unzip(dest, g.NativeDir, []string{"META-INF/"}); e != nil {
				return g, e
			}
		}
		if n := l.Natives["windows"]; n != "" {
			n = strings.ReplaceAll(n, "${arch}", "64")
			nd := l.Downloads.Classifiers[n]
			if nd.URL != "" {
				np := filepath.Join(root, "libraries", filepath.FromSlash(nd.Path))
				if e = download(nd, np); e != nil {
					return g, e
				}
				if e = unzip(np, g.NativeDir, l.Extract.Exclude); e != nil {
					return g, e
				}
			}
		}
		if i%12 == 0 {
			status(fmt.Sprintf("Kütüphaneler %d/%d", i+1, len(libs)))
		}
	}
	jar := filepath.Join(root, "versions", mcVersion, mcVersion+".jar")
	if e = download(v.Downloads["client"], jar); e != nil {
		return g, e
	}
	g.Classpath = append(g.Classpath, jar)
	idx := filepath.Join(root, "assets", "indexes", g.AssetID+".json")
	if e = download(v.AssetIndex, idx); e != nil {
		return g, e
	}
	var ai AssetIndex
	b, e := os.ReadFile(idx)
	if e != nil {
		return g, e
	}
	if e = json.Unmarshal(b, &ai); e != nil {
		return g, e
	}
	type job struct {
		Hash string
		Size int64
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	done := 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if len(j.Hash) != 40 {
					mu.Lock()
					if first == nil {
						first = errors.New("Geçersiz asset hash")
					}
					mu.Unlock()
					continue
				}
				mu.Lock()
				failed := first != nil
				mu.Unlock()
				if failed {
					continue
				}
				prefix := j.Hash[:2] + "/" + j.Hash
				p := filepath.Join(root, "assets", "objects", filepath.FromSlash(prefix))
				e := download(Download{URL: "https://resources.download.minecraft.net/" + prefix, SHA1: j.Hash, Size: j.Size}, p)
				mu.Lock()
				if e != nil && first == nil {
					first = e
				}
				done++
				if done%50 == 0 {
					status(fmt.Sprintf("Oyun dosyaları %d/%d", done, len(ai.Objects)))
				}
				mu.Unlock()
			}
		}()
	}
	unique := map[string]bool{}
	for _, o := range ai.Objects {
		if !unique[o.Hash] {
			unique[o.Hash] = true
			jobs <- job{o.Hash, o.Size}
		}
	}
	close(jobs)
	wg.Wait()
	if first != nil {
		return g, first
	}
	g.Java, e = findJava(root, java, status)
	if e != nil {
		return g, e
	}
	status("Minecraft + Fabric hazır.")
	return g, nil
}
func expand(raw []json.RawMessage, values map[string]string) ([]string, error) {
	var out []string
	for _, r := range raw {
		var s string
		if json.Unmarshal(r, &s) == nil {
			out = append(out, s)
			continue
		}
		var obj struct {
			Rules []Rule
			Value json.RawMessage
		}
		if e := json.Unmarshal(r, &obj); e != nil {
			return nil, e
		}
		if !allowed(obj.Rules) {
			continue
		}
		if json.Unmarshal(obj.Value, &s) == nil {
			out = append(out, s)
			continue
		}
		var ss []string
		if e := json.Unmarshal(obj.Value, &ss); e != nil {
			return nil, e
		}
		out = append(out, ss...)
	}
	for i, s := range out {
		for k, v := range values {
			s = strings.ReplaceAll(s, "${"+k+"}", v)
		}
		if strings.Contains(s, "${") {
			return nil, fmt.Errorf("Desteklenmeyen oyun parametresi: %s", s)
		}
		out[i] = s
	}
	return out, nil
}
func launch(g Game, a Account, c Config, status func(string)) error {
	if a.Token == "" || time.Now().After(a.Expires) {
		return errors.New("Önce Minecraft hesabınızla giriş yapın.")
	}
	ram := c.RAM
	if ram < 2 || ram > 32 {
		ram = 4
	}
	v := map[string]string{"natives_directory": g.NativeDir, "launcher_name": "ChaosLauncher", "launcher_version": launcherVersion, "classpath": strings.Join(g.Classpath, string(os.PathListSeparator)), "classpath_separator": string(os.PathListSeparator), "library_directory": filepath.Join(g.Root, "libraries"), "auth_player_name": a.Name, "version_name": g.Version, "game_directory": g.Root, "assets_root": filepath.Join(g.Root, "assets"), "assets_index_name": g.AssetID, "auth_uuid": a.ID, "auth_access_token": a.Token, "clientid": c.ClientID, "auth_xuid": a.XUID, "user_type": "msa", "version_type": "release", "user_properties": "{}", "game_assets": filepath.Join(g.Root, "assets")}
	j, e := expand(g.JVM, v)
	if e != nil {
		return e
	}
	args, e := expand(g.Args, v)
	if e != nil {
		return e
	}
	j = append([]string{"-Xms1G", fmt.Sprintf("-Xmx%dG", ram)}, j...)
	j = append(j, g.Main)
	j = append(j, args...)
	cmd := exec.Command(g.Java, j...)
	configureProcess(cmd)
	cmd.Dir = g.Root
	log, e := os.Create(filepath.Join(g.Root, "logs", "launcher-game.log"))
	if e != nil {
		return e
	}
	defer log.Close()
	cmd.Stdout = log
	cmd.Stderr = log
	if e = cmd.Start(); e != nil {
		return e
	}
	status("Minecraft çalışıyor • " + a.Name)
	e = cmd.Wait()
	if e != nil {
		return fmt.Errorf("Minecraft kapandı: %v. logs/launcher-game.log dosyasına bakın", e)
	}
	status("Minecraft kapatıldı.")
	return nil
}
func copyMods(from, to string) (int, error) {
	if e := mkdir(to); e != nil {
		return 0, e
	}
	if s, e := os.Stat(filepath.Join(from, "mods")); e == nil && s.IsDir() {
		from = filepath.Join(from, "mods")
	}
	if filepath.Clean(from) == filepath.Clean(to) {
		return 0, errors.New("Kaynak ve hedef aynı klasör")
	}
	entries, e := os.ReadDir(from)
	if e != nil {
		return 0, e
	}
	n := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".jar") {
			continue
		}
		src, e := os.Open(filepath.Join(from, entry.Name()))
		if e != nil {
			return n, e
		}
		dst, e := os.OpenFile(filepath.Join(to, entry.Name()), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(e) {
			src.Close()
			continue
		}
		if e != nil {
			src.Close()
			return n, e
		}
		_, e = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if e != nil {
			os.Remove(filepath.Join(to, entry.Name()))
			return n, e
		}
		n++
	}
	return n, nil
}
func configLoad(root string) Config {
	c := Config{RAM: 4}
	b, e := os.ReadFile(filepath.Join(root, "launcher.json"))
	if e == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}
func configSave(root string, c Config) error {
	if e := mkdir(root); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(root, "launcher.json"), b, 0600)
}

var _ = runtime.GOOS

func checkChaos(dir string) error {
	entries, e := os.ReadDir(dir)
	if e != nil {
		return errors.New("Önce TikTok Chaos modunu içe aktarın.")
	}
	count := 0
	for _, f := range entries {
		if f.IsDir() || !strings.EqualFold(filepath.Ext(f.Name()), ".jar") {
			continue
		}
		r, e := zip.OpenReader(filepath.Join(dir, f.Name()))
		if e != nil {
			continue
		}
		for _, z := range r.File {
			if z.Name != "fabric.mod.json" {
				continue
			}
			src, e := z.Open()
			if e != nil {
				continue
			}
			var m struct{ ID string }
			e = json.NewDecoder(io.LimitReader(src, 1<<20)).Decode(&m)
			src.Close()
			if e == nil && m.ID == "tiktokchaos" {
				count++
			}
			break
		}
		r.Close()
	}
	if count == 0 {
		return errors.New("TikTok Chaos JAR dosyası mods klasöründe bulunamadı.")
	}
	if count > 1 {
		return errors.New("Birden fazla TikTok Chaos sürümü bulundu. Mods klasöründe tek sürüm bırakın.")
	}
	return nil
}
