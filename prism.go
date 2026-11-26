package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const prismRepo = "PrismLauncher/PrismLauncher"
const instanceID = "TikTokChaos"

func prismData(root string) string   { return filepath.Join(root, "prism-data") }
func instanceDir(root string) string { return filepath.Join(prismData(root), "instances", instanceID) }
func gameDir(root string) string     { return filepath.Join(instanceDir(root), ".minecraft") }
func modsDir(root string) string     { return filepath.Join(gameDir(root), "mods") }
func prismBinary(dir string) string {
	var found string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "prismlauncher.exe") {
			found = p
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

var portableName = regexp.MustCompile(`(?i)^PrismLauncher-Windows-MinGW-(?:w64|x64)-Portable-[0-9.]+\.zip$`)

func ensurePrism(root string, status func(string)) (string, error) {
	target := filepath.Join(root, "prism-runtime")
	if p := prismBinary(target); p != "" {
		return p, nil
	}
	status("Prism Launcher resmi sürümü kontrol ediliyor…")
	var r Release
	if e := getJSON("https://api.github.com/repos/"+prismRepo+"/releases/latest", &r); e != nil {
		return "", e
	}
	var a ReleaseAsset
	for _, v := range r.Assets {
		if portableName.MatchString(v.Name) {
			a = v
			break
		}
	}
	if a.Name == "" {
		return "", errors.New("Resmi Windows x64 Prism Portable paketi bulunamadı.")
	}
	tmp, e := os.MkdirTemp(root, "prism-install-")
	if e != nil {
		return "", e
	}
	defer os.RemoveAll(tmp)
	p := filepath.Join(tmp, "prism.zip")
	status("Prism Portable indiriliyor ve SHA-256 doğrulanıyor…")
	if e = fetchAsset(prismRepo, r.Tag, a, p); e != nil {
		return "", e
	}
	unpack := filepath.Join(tmp, "files")
	if e = mkdir(unpack); e != nil {
		return "", e
	}
	if e = unzip(p, unpack, nil); e != nil {
		return "", e
	}
	if prismBinary(unpack) == "" {
		return "", errors.New("Prism EXE arşivde yok.")
	}
	if _, e = os.Stat(target); e == nil {
		return "", errors.New("Eksik Prism klasörü mevcut. Silmeden önce yedekleyin: " + target)
	}
	if e = os.Rename(unpack, target); e != nil {
		return "", e
	}
	return prismBinary(target), nil
}
func copyMissing(src, dst string) error {
	if _, e := os.Stat(src); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		rel, e := filepath.Rel(src, p)
		if e != nil {
			return e
		}
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return mkdir(to)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if e = mkdir(filepath.Dir(to)); e != nil {
			return e
		}
		in, e := os.Open(p)
		if e != nil {
			return e
		}
		defer in.Close()
		out, e := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if os.IsExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		_, e = io.Copy(out, in)
		ce := out.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			os.Remove(to)
		}
		return e
	})
}
func initInstance(root string) error {
	if e := mkdir(modsDir(root)); e != nil {
		return e
	}
	d := instanceDir(root)
	p := filepath.Join(d, "mmc-pack.json")
	if _, e := os.Stat(p); os.IsNotExist(e) {
		b, _ := json.MarshalIndent(map[string]any{"formatVersion": 1, "components": []map[string]any{{"uid": "net.minecraft", "version": mcVersion, "important": true}, {"uid": "net.fabricmc.fabric-loader", "version": fabricVersion}}}, "", "  ")
		if e = os.WriteFile(p, b, 0644); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	p = filepath.Join(d, "instance.cfg")
	if _, e := os.Stat(p); os.IsNotExist(e) {
		if e = os.WriteFile(p, []byte("InstanceType=OneSix\nname=TikTok Chaos\niconKey=default\n"), 0644); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	marker := filepath.Join(d, "chaos-migrated-v04")
	if _, e := os.Stat(marker); os.IsNotExist(e) {
		for _, n := range []string{"mods", "saves", "config", "resourcepacks", "shaderpacks", "screenshots", "options.txt", "servers.dat"} {
			if e = copyMissing(filepath.Join(root, "game", n), filepath.Join(gameDir(root), n)); e != nil {
				return e
			}
		}
		return os.WriteFile(marker, []byte("ok"), 0600)
	} else {
		return e
	}
}
func setInstanceOptions(root string, c Config, java string) error {
	p := filepath.Join(instanceDir(root), "instance.cfg")
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	if c.RAM < 2 || c.RAM > 32 {
		return errors.New("RAM 2–32 GB olmalı")
	}
	vals := map[string]string{"OverrideMemory": "true", "MinMemAlloc": "1024", "MaxMemAlloc": fmt.Sprint(c.RAM * 1024), "OverrideJavaLocation": "true", "JavaPath": filepath.ToSlash(java)}
	lines := strings.Split(strings.TrimRight(string(b), "\r\n"), "\n")
	for i, l := range lines {
		key, _, _ := strings.Cut(l, "=")
		if v, ok := vals[key]; ok {
			lines[i] = key + "=" + v
			delete(vals, key)
		}
	}
	for k, v := range vals {
		lines = append(lines, k+"="+v)
	}
	return os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
func runPrism(root string, c Config, play bool, status func(string)) error {
	if e := initInstance(root); e != nil {
		return e
	}
	p, e := ensurePrism(root, status)
	if e != nil {
		return e
	}
	args := []string{"--dir", prismData(root)}
	if play {
		if e = checkChaos(modsDir(root)); e != nil {
			return e
		}
		running, game, scanErr := chaosProcesses(root)
		if scanErr == nil && game {
			return errors.New("Chaos Minecraft zaten çalışıyor.")
		}
		if scanErr == nil && !running {
			java, e := findJava(filepath.Join(root, "game"), c.Java, status)
			if e != nil {
				return e
			}
			if e = setInstanceOptions(root, c, java); e != nil {
				return e
			}
		} else {
			status("Prism mevcut ayarlarıyla başlatılıyor; RAM/Java değişiklikleri sonraki kapalı oturumda uygulanır.")
		}
		args = append(args, "--launch", instanceID)
	}
	cmd := exec.Command(p, args...)
	cmd.Dir = filepath.Dir(p)
	configureProcess(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	go cmd.Wait()
	if play {
		status("Minecraft başlatma isteği Prism'e gönderildi. İlk açılışta indirmeler olabilir.")
	} else {
		status("Prism açılıyor. Hesaplar > Microsoft hesabı ekle.")
	}
	return nil
}
