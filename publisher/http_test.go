package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestOAuthFlowMock(t *testing.T) {
	s := Service{File: filepath.Join(t.TempDir(), "state.json"), Bindings: map[string]Binding{}, Pending: map[string]Pending{}, Results: map[string]Status{}, Key: "test", Redirect: "https://example.org/callback", Verify: func(code string) (string, string, error) { return "same-open-id", "streamer", nil }}
	h := s.handler()
	start := func(device string) string {
		r := httptest.NewRequest("POST", "/start", nil)
		r.Header.Set("Authorization", "Bearer "+device)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		var v map[string]string
		json.Unmarshal(w.Body.Bytes(), &v)
		u, e := url.Parse(v["URL"])
		if e != nil || u.Host != "www.tiktok.com" {
			t.Fatal(v, e)
		}
		return u.Query().Get("state")
	}
	callback := func(state string) int {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/callback?state="+state+"&code=mock", nil))
		return w.Code
	}
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	x := start(a)
	if callback(x) != 200 || callback(x) != 400 {
		t.Fatal("state replay check failed")
	}
	if callback(start(b)) != 200 {
		t.Fatal("second callback failed")
	}
	r := httptest.NewRequest(http.MethodGet, "/status", nil)
	r.Header.Set("Authorization", "Bearer "+a)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var v Status
	json.Unmarshal(w.Body.Bytes(), &v)
	if v.State != "verified" || v.Warnings != 1 {
		t.Fatal(v)
	}
	r = httptest.NewRequest(http.MethodGet, "/status", nil)
	r.Header.Set("Authorization", "Bearer "+b)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	json.Unmarshal(w.Body.Bytes(), &v)
	if v.State != "denied" {
		t.Fatal(v)
	}
}
