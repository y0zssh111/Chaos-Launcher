//go:build !windows

package main

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
func main()                          {}

func chaosProcesses(root string) (bool, bool, error) { return false, false, nil }

func configurePrismProcess(cmd *exec.Cmd) {}
func showPrismAccountWindow(root string)  {}
