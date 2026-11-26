package main

import "testing"

func TestOtherJavaAndPrismIgnored(t *testing.T) {
	p, g, e := classifyProcesses(`C:\Chaos`, []byte(`[{"Name":"javaw.exe","ExecutablePath":"C:\\Other\\javaw.exe","ProcessId":1},{"Name":"prismlauncher.exe","ExecutablePath":"C:\\Other\\prismlauncher.exe","ProcessId":2}]`))
	if p || g || e != nil {
		t.Fatal(p, g, e)
	}
}
func TestOwnPrism(t *testing.T) {
	p, g, e := classifyProcesses(`C:\Chaos`, []byte(`{"Name":"prismlauncher.exe","ExecutablePath":"C:\\Chaos\\prism-runtime\\prismlauncher.exe","ProcessId":2}`))
	if !p || g || e != nil {
		t.Fatal(p, g, e)
	}
}
func TestOwnJava(t *testing.T) {
	p, g, e := classifyProcesses(`C:\Chaos`, []byte(`[{"Name":"prismlauncher.exe","ExecutablePath":"C:\\Chaos\\prism-runtime\\prismlauncher.exe","ProcessId":2},{"Name":"javaw.exe","ExecutablePath":"C:\\Java\\javaw.exe","ParentProcessId":2,"ProcessId":3}]`))
	if !p || !g || e != nil {
		t.Fatal(p, g, e)
	}
}
func TestOrphanJava(t *testing.T) {
	p, g, e := classifyProcesses(`C:\Chaos`, []byte(`{"Name":"javaw.exe","CommandLine":"java --gameDir C:\\Chaos\\prism-data\\instances\\TikTokChaos\\.minecraft"}`))
	if p || !g || e != nil {
		t.Fatal(p, g, e)
	}
}
