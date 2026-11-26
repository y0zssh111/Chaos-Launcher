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
func classifyProcesses(root string, data []byte) (bool, bool, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	if len(data) == 0 || string(data) == "null" {
		return false, false, nil
	}
	var ps []localProcess
	if data[0] == '{' {
		var p localProcess
		if e := json.Unmarshal(data, &p); e != nil {
			return false, false, e
		}
		ps = append(ps, p)
	} else if e := json.Unmarshal(data, &ps); e != nil {
		return false, false, fmt.Errorf("Chaos süreç bilgisi okunamadı: %w", e)
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
