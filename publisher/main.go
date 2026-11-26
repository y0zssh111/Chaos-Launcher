// Publisher verification service. Run behind HTTPS; credentials are server-only.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Binding struct {
	Device, Username string
	Warnings         int
	LastWarning      string
}
type Pending struct {
	Device  string
	Expires time.Time
}
type Status struct {
	State, Username, Message string
	Warnings                 int
	LastWarning              string
}
type Service struct {
	mu                          sync.Mutex
	Bindings                    map[string]Binding
	Pending                     map[string]Pending
	Results                     map[string]Status
	File, Key, Secret, Redirect string
	Verify                      func(string) (string, string, error)
}

func random() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func hash(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }
func (s *Service) persist() error {
	b, e := json.Marshal(s.Bindings)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(filepath.Dir(s.File), "bindings-*")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if _, e = tmp.Write(b); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Sync(); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	return os.Rename(tmp.Name(), s.File)
}
func (s *Service) bind(device, id, user string) (Status, error) {
	key := hash(id)
	for k, b := range s.Bindings {
		if b.Device == device && k != key {
			return Status{State: "denied", Message: "Bu kurulumda başka bir yayıncı zaten bağlı."}, nil
		}
	}
	if b, ok := s.Bindings[key]; ok {
		if b.Device != device {
			old := b
			b.Warnings++
			b.LastWarning = time.Now().UTC().Format(time.RFC3339)
			s.Bindings[key] = b
			if e := s.persist(); e != nil {
				s.Bindings[key] = old
				return Status{}, e
			}
			return Status{State: "denied", Message: "Bu TikTok hesabı başka bir kuruluma bağlı. Yayıncıya uyarı kaydedildi."}, nil
		}
		return Status{State: "verified", Username: b.Username, Warnings: b.Warnings, LastWarning: b.LastWarning}, nil
	}
	s.Bindings[key] = Binding{Device: device, Username: user}
	if e := s.persist(); e != nil {
		delete(s.Bindings, key)
		return Status{}, e
	}
	return Status{State: "verified", Username: user}, nil
}
func (s *Service) status(device string) Status {
	for _, b := range s.Bindings {
		if b.Device == device {
			return Status{State: "verified", Username: b.Username, Warnings: b.Warnings, LastWarning: b.LastWarning}
		}
	}
	if v, ok := s.Results[device]; ok {
		return v
	}
	return Status{State: "unbound"}
}
func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}
func device(r *http.Request) string {
	v := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	b, e := hex.DecodeString(v)
	if e != nil || len(b) != 32 {
		return ""
	}
	return hash(v)
}
func (s *Service) handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		d := device(r)
		if r.Method != "POST" || d == "" {
			http.Error(w, "unauthorized", 401)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if v := s.status(d); v.State == "verified" {
			write(w, v)
			return
		}
		for k, p := range s.Pending {
			if time.Now().After(p.Expires) || p.Device == d {
				delete(s.Pending, k)
			}
		}
		if len(s.Pending) >= 1000 {
			http.Error(w, "busy", 429)
			return
		}
		state, e := random()
		if e != nil {
			http.Error(w, "random failure", 500)
			return
		}
		s.Pending[state] = Pending{d, time.Now().Add(10 * time.Minute)}
		delete(s.Results, d)
		q := url.Values{"client_key": {s.Key}, "scope": {"user.info.basic,user.info.profile"}, "response_type": {"code"}, "redirect_uri": {s.Redirect}, "state": {state}}
		write(w, map[string]string{"State": "pending", "URL": "https://www.tiktok.com/v2/auth/authorize/?" + q.Encode()})
	})
	m.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		d := device(r)
		if r.Method != "GET" || d == "" {
			http.Error(w, "unauthorized", 401)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		write(w, s.status(d))
	})
	m.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		q := r.URL.Query()
		s.mu.Lock()
		p, ok := s.Pending[q.Get("state")]
		delete(s.Pending, q.Get("state"))
		s.mu.Unlock()
		if !ok || time.Now().After(p.Expires) {
			http.Error(w, "Invalid or expired state", 400)
			return
		}
		id, user, e := s.Verify(q.Get("code"))
		s.mu.Lock()
		defer s.mu.Unlock()
		if e != nil {
			s.Results[p.Device] = Status{State: "denied", Message: "TikTok doğrulaması tamamlanmadı."}
			http.Error(w, "Verification failed. Return to launcher.", 400)
			return
		}
		v, e := s.bind(p.Device, id, user)
		if e != nil {
			http.Error(w, "Storage failure", 500)
			return
		}
		if len(s.Results) >= 2000 {
			s.Results = map[string]Status{}
		}
		s.Results[p.Device] = v
		fmt.Fprint(w, "İşlem tamamlandı. Launcher uygulamasına dönün.")
	})
	return m
}
func (s *Service) verify(code string) (string, string, error) {
	if code == "" {
		return "", "", errors.New("no code")
	}
	c := &http.Client{Timeout: 20 * time.Second}
	r, e := c.PostForm("https://open.tiktokapis.com/v2/oauth/token/", url.Values{"client_key": {s.Key}, "client_secret": {s.Secret}, "code": {code}, "grant_type": {"authorization_code"}, "redirect_uri": {s.Redirect}})
	if e != nil {
		return "", "", e
	}
	var t struct {
		Access string `json:"access_token"`
		OpenID string `json:"open_id"`
	}
	e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&t)
	r.Body.Close()
	if e != nil || r.StatusCode != 200 || t.Access == "" || t.OpenID == "" {
		return "", "", errors.New("token failed")
	}
	req, _ := http.NewRequest("GET", "https://open.tiktokapis.com/v2/user/info/?fields=open_id,username", nil)
	req.Header.Set("Authorization", "Bearer "+t.Access)
	r, e = c.Do(req)
	if e != nil {
		return "", "", e
	}
	defer r.Body.Close()
	var v struct {
		Data struct {
			User struct {
				OpenID   string `json:"open_id"`
				Username string `json:"username"`
			}
		}
	}
	e = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&v)
	u := v.Data.User
	if e != nil || r.StatusCode != 200 || u.OpenID != t.OpenID || u.Username == "" {
		return "", "", errors.New("profile failed")
	}
	return u.OpenID, u.Username, nil
}
func main() {
	s := &Service{Bindings: map[string]Binding{}, Pending: map[string]Pending{}, Results: map[string]Status{}, File: os.Getenv("CHAOS_BINDINGS_FILE"), Key: os.Getenv("TIKTOK_CLIENT_KEY"), Secret: os.Getenv("TIKTOK_CLIENT_SECRET"), Redirect: os.Getenv("TIKTOK_REDIRECT_URI")}
	if s.File == "" {
		s.File = "bindings.json"
	}
	if s.Key == "" || s.Secret == "" || !strings.HasPrefix(s.Redirect, "https://") {
		log.Fatal("Set TikTok credentials and HTTPS callback URL")
	}
	b, e := os.ReadFile(s.File)
	if e == nil {
		if e = json.Unmarshal(b, &s.Bindings); e != nil {
			log.Fatal(e)
		}
	} else if !os.IsNotExist(e) {
		log.Fatal(e)
	}
	if s.Bindings == nil {
		log.Fatal("invalid binding store")
	}
	s.Verify = s.verify
	addr := os.Getenv("CHAOS_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	log.Fatal((&http.Server{Addr: addr, Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}).ListenAndServe())
}
