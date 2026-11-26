package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type localProcess struct {
	Name, ExecutablePath, CommandLine string
	ProcessId, ParentProcessId        int
}

func normWin(s string) string { return strings.ToLower(strings.ReplaceAll(s, "/", `\`)) }
func decodeProcesses(data []byte) ([]localProcess, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	if len(data) == 0 || string(data) == "null" {
		return nil, nil
	}
	var ps []localProcess
	if data[0] == '{' {
		var p localProcess
		if e := json.Unmarshal(data, &p); e != nil {
			return nil, e
		}
		ps = append(ps, p)
	} else if e := json.Unmarshal(data, &ps); e != nil {
		return nil, fmt.Errorf("Chaos süreç bilgisi okunamadı: %w", e)
	}
	return ps, nil
}
func classifyProcesses(root string, data []byte) (bool, bool, error) {
	ps, e := decodeProcesses(data)
	if e != nil {
		return false, false, e
	}
	r := strings.TrimRight(normWin(root), `\`) + `\`
	prismIDs := map[int]bool{}
	for _, p := range ps {
		if strings.EqualFold(p.Name, "prismlauncher.exe") && strings.HasPrefix(normWin(p.ExecutablePath), r+`prism-runtime\`) {
			prismIDs[p.ProcessId] = true
		}
	}
	for _, p := range ps {
		if strings.EqualFold(p.Name, "java.exe") || strings.EqualFold(p.Name, "javaw.exe") {
			if prismIDs[p.ParentProcessId] || strings.Contains(normWin(p.CommandLine), r+`prism-data\instances\tiktokchaos\`) || strings.HasPrefix(normWin(p.ExecutablePath), r) {
				return len(prismIDs) > 0, true, nil
			}
		}
	}
	return len(prismIDs) > 0, false, nil
}

// A real visible GLFW window must belong to a Java process scoped to this instance.
func ownsGameWindow(root string, data []byte, ids map[int]bool) bool {
	ps, e := decodeProcesses(data)
	if e != nil {
		return false
	}
	r := strings.TrimRight(normWin(root), `\`) + `\`
	prism := map[int]bool{}
	for _, p := range ps {
		if strings.EqualFold(p.Name, "prismlauncher.exe") && strings.HasPrefix(normWin(p.ExecutablePath), r+`prism-runtime\`) {
			prism[p.ProcessId] = true
		}
	}
	for _, p := range ps {
		if !ids[p.ProcessId] || !(strings.EqualFold(p.Name, "java.exe") || strings.EqualFold(p.Name, "javaw.exe")) {
			continue
		}
		if prism[p.ParentProcessId] || strings.Contains(normWin(p.CommandLine), r+`prism-data\instances\tiktokchaos\`) {
			return true
		}
	}
	return false
}
