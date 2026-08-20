// Command kustofmt formats YAML in kustomize's emitted style.
//
// Usage:
//
//	kustofmt [flags] [path ...]
//
// With no paths it reads standard input and writes to standard output, so it
// composes in a pipe. With paths, it formats files and directories; directories
// are walked recursively for *.yaml and *.yml.
//
// The flags mirror gofmt:
//
//	-l    list files whose formatting differs (exit 1 if any)
//	-w    write the result back to the file
//	-d    print unified diffs
//
// Files encrypted with sops are skipped by default; see --include-sops.
package main

import (
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
