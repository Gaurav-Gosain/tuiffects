//go:build !unix

package main

import "os"

// terminalSize has no portable answer off unix, so the caller falls back to
// the environment and then to eighty by twenty four.
func terminalSize(*os.File) (width, height int, ok bool) { return 0, 0, false }

// isTerminal falls back to the file's mode off unix. It says yes to a
// character device that is not a terminal, /dev/null among them, which is the
// reason the unix build asks the kernel instead.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
