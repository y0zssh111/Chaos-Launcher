package main

import (
	"archive/zip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ReleaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}
type Release struct {
	Tag    string         `json:"tag_name"`
	Draft  bool           `json:"draft"`
	Pre    bool           `json:"prerelease"`
	Assets []ReleaseAsset `json:"assets"`
}
type Update struct {
	Version, Repo, Tag string
	Asset              ReleaseAsset
}

var updateRepo = "y0zssh111/Chaos-Launcher" // Official update repository.
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func releaseRepo(root string) string {
	if updateRepo != "" {
		return updateRepo
	}
	b, _ := os.ReadFile(filepath.Join(root, "updates-repo.txt"))
	return strings.TrimSpace(string(b))
}
func fetchAsset(repo, tag string, a ReleaseAsset, p string) error {
	if !repoPattern.MatchString(repo) || strings.ContainsAny(tag, "/\\") || filepath.Base(a.Name) != a.Name || a.Name == "" || a.URL != "https://github.com/"+repo+"/releases/download/"+tag+"/"+a.Name {
		return errors.New("Geçersiz GitHub dosya adresi")
	}
	sum := strings.TrimPrefix(a.Digest, "sha256:")
	d, e := hex.DecodeString(sum)
	if e != nil || len(d) != 32 || !strings.HasPrefix(a.Digest, "sha256:") || a.Size <= 0 || a.Size > 1<<30 {
		return errors.New("GitHub SHA-256 veya dosya boyutu eksik/geçersiz")
	}
	if e = download(Download{URL: a.URL, Size: a.Size}, p); e != nil {
		return e
	}
	if e = verifySHA256(p, sum); e != nil {
		os.Remove(p)
		return e
	}
	return nil
}
func versionParts(s string) ([3]int, bool, bool) {
	var a [3]int
	v, _, pre := strings.Cut(s, "-")
	p := strings.Split(v, ".")
	if len(p) != 3 {
		return a, false, false
	}
	for i, x := range p {
		n, e := strconv.Atoi(x)
		if e != nil || n < 0 || n > 100000 {
			return a, false, false
		}
		a[i] = n
	}
	return a, pre, true
}
func newer(a, b string) bool {
	x, p, ok := versionParts(a)
	y, q, ok2 := versionParts(b)
	if !ok || !ok2 {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return !p && q
}
func findUpdate(rels []Release, repo, kind, current string) *Update {
	var best *Update
	for _, r := range rels {
		if r.Draft || r.Pre || !strings.HasPrefix(r.Tag, kind+"-v") {
			continue
		}
		v := strings.TrimPrefix(r.Tag, kind+"-v")
		_, pre, ok := versionParts(v)
		if !ok || pre || !newer(v, current) {
			continue
		}
		name := "TikTokChaosLauncher.exe"
		if kind == "mod" {
			name = "tiktokchaos.jar"
		}
		for _, a := range r.Assets {
			if a.Name == name && (best == nil || newer(v, best.Version)) {
				best = &Update{v, repo, r.Tag, a}
			}
		}
	}
	return best
}
func availableUpdates(root string) (*Update, *Update, error) {
	repo := releaseRepo(root)
	if !repoPattern.MatchString(repo) {
		return nil, nil, errors.New("Güncelleme deposu ayarlanmadı. Geliştirici GitHub owner/repo adresini updates-repo.txt dosyasına yazmalı.")
	}
	var rel []Release
	if e := getJSON("https://api.github.com/repos/"+repo+"/releases?per_page=100", &rel); e != nil {
		return nil, nil, e
	}
	v := "0.0.0"
	paths, e := chaosJars(modsDir(root))
	if e != nil {
		return nil, nil, e
	}
	for _, p := range paths {
		_, x := jarMeta(p)
		if _, _, ok := versionParts(x); ok && newer(x, v) {
			v = x
		}
	}
	return findUpdate(rel, repo, "launcher", launcherVersion), findUpdate(rel, repo, "mod", v), nil
}
func jarMeta(p string) (string, string) {
	r, e := zip.OpenReader(p)
	if e != nil {
		return "", ""
	}
	defer r.Close()
	for _, f := range r.File {
		if f.Name == "fabric.mod.json" {
			q, e := f.Open()
			if e != nil {
				return "", ""
			}
			defer q.Close()
			var m struct{ ID, Version string }
			if json.NewDecoder(io.LimitReader(q, 1<<20)).Decode(&m) == nil {
				return m.ID, m.Version
			}
		}
	}
	return "", ""
}
func chaosJars(dir string) ([]string, error) {
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out []string
	for _, f := range entries {
		if !f.IsDir() && strings.EqualFold(filepath.Ext(f.Name()), ".jar") {
			p := filepath.Join(dir, f.Name())
			id, _ := jarMeta(p)
			if id == "tiktokchaos" {
				out = append(out, p)
			}
		}
	}
	return out, nil
}
func replaceMod(root, staged, version string) error {
	if _, pre, ok := versionParts(version); !ok || pre || !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) {
		return errors.New("Geçersiz mod sürümü")
	}
	id, v := jarMeta(staged)
	if id != "tiktokchaos" || v != version {
		return errors.New("Yeni JAR kimliği veya sürümü uyuşmuyor")
	}
	dir := modsDir(root)
	if e := mkdir(dir); e != nil {
		return e
	}
	old, e := chaosJars(dir)
	if e != nil {
		return e
	}
	target := filepath.Join(dir, "tiktokchaos-"+version+".jar")
	if _, e = os.Stat(target); e == nil {
		return errors.New("Hedef JAR zaten var; dosya değiştirilmedi")
	}
	backup := filepath.Join(root, "mod-backups", fmt.Sprint(time.Now().UnixNano()))
	if e = mkdir(backup); e != nil {
		return e
	}
	var moved []string
	rollback := func() {
		for _, p := range moved {
			_ = os.Rename(filepath.Join(backup, filepath.Base(p)), p)
		}
	}
	for _, p := range old {
		if e = os.Rename(p, filepath.Join(backup, filepath.Base(p))); e != nil {
			rollback()
			return e
		}
		moved = append(moved, p)
	}
	if e = os.Rename(staged, target); e != nil {
		rollback()
		return e
	}
	return nil
}
