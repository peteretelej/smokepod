// Command stub is a deterministic smokepod target used by tests and docs.
// It answers the probes `smokepod init` runs (--version, --help) plus a
// couple of commands, so init/record/verify roundtrips have stable output.
package main

import (
	"fmt"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: stub [command]")
		os.Exit(2)
	}
	switch args[0] {
	case "--version":
		fmt.Println("stub version 1.2.3")
	case "--help":
		fmt.Println("usage: stub [command]")
	case "greet":
		fmt.Println("hello from stub")
	case "fail":
		fmt.Fprintln(os.Stderr, "stub failed")
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(2)
	}
}
