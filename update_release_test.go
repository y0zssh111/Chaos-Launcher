package main

import "testing"

func TestOfficialUpdateUpgrade(t *testing.T) {
	repo := releaseRepo(t.TempDir())
	if repo != "y0zssh111/Chaos-Launcher" {
		t.Fatal(repo)
	}
	rels := []Release{
		{Tag: "launcher-v0.4.2", Assets: []ReleaseAsset{{Name: "TikTokChaosLauncher.exe"}}},
		{Tag: "launcher-v0.4.3", Assets: []ReleaseAsset{{Name: "TikTokChaosLauncher.exe"}}},
		{Tag: "launcher-v0.4.4", Assets: []ReleaseAsset{{Name: "wrong.exe"}}},
		{Tag: "launcher-v0.4.5", Pre: true, Assets: []ReleaseAsset{{Name: "TikTokChaosLauncher.exe"}}},
	}
	v := findUpdate(rels, repo, "launcher", "0.4.2")
	if v == nil || v.Version != "0.4.3" || v.Repo != repo {
		t.Fatal(v)
	}
	if v := findUpdate(rels, repo, "launcher", "0.4.3"); v != nil {
		t.Fatal(v)
	}
}
