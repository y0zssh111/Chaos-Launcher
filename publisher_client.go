package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var microsoftClientID string // configured at build time by project owner
var publisherURL string      // configured at build time by project owner
const creatorUser = "y0zshhiii"
const creatorName = "YσZSHI帝"

type PublisherStatus struct {
	State, Username, Message, URL, LastWarning string
	Warnings                                   int
}

func projectClientID() string {
	if microsoftClientID != "" {
		return microsoftClientID
	}
	return strings.TrimSpace(os.Getenv("CHAOS_MS_CLIENT_ID"))
}
func publisherEndpoint() string {
	if publisherURL != "" {
		return publisherURL
	}
	return strings.TrimSpace(os.Getenv("CHAOS_PUBLISHER_URL"))
}
func deviceKey(root string) (string, error) {
	p := filepath.Join(root, "publisher.key")
	b, e := os.ReadFile(p)
	if e == nil {
		v := strings.TrimSpace(string(b))
		d, e := hex.DecodeString(v)
		if e == nil && len(d) == 32 {
			return v, nil
		}
		return "", errors.New("Yayıncı anahtarı bozuk; destekle iletişim kurun.")
	}
	if !os.IsNotExist(e) {
		return "", e
	}
	d := make([]byte, 32)
	if _, e = rand.Read(d); e != nil {
		return "", e
	}
	v := hex.EncodeToString(d)
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(e) {
		return deviceKey(root)
	}
	if e != nil {
		return "", e
	}
	_, e = f.WriteString(v)
	ce := f.Close()
	if e != nil {
		return "", e
	}
	return v, ce
}
func publisherRequest(root, route string) (PublisherStatus, error) {
	var v PublisherStatus
	ep := strings.TrimRight(publisherEndpoint(), "/")
	u, e := url.Parse(ep)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return v, errors.New("TikTok doğrulama sunucusu henüz yapılandırılmadı.")
	}
	k, e := deviceKey(root)
	if e != nil {
		return v, e
	}
	method := "GET"
	if route == "start" {
		method = "POST"
	}
	req, e := http.NewRequest(method, ep+"/"+route, nil)
	if e != nil {
		return v, e
	}
	req.Header.Set("Authorization", "Bearer "+k)
	c := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, e := c.Do(req)
	if e != nil {
		return v, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return v, fmt.Errorf("Yayıncı sunucusu HTTP %d", r.StatusCode)
	}
	e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&v)
	return v, e
}
