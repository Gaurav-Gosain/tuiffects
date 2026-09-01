//go:build unix

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// winsize asks the kernel about the window on a file.
//
// It asks about the file the animation is going to, not standard input, because
// standard input is a pipe every time this program is worth running.
func winsize(f *os.File) (cols, rows int, ok bool) {
	var ws struct{ rows, cols, xpixel, ypixel uint16 }
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0, 0, false
	}
	return int(ws.cols), int(ws.rows), true
}

// terminalSize is the window size, when there is a window.
func terminalSize(f *os.File) (width, height int, ok bool) {
	cols, rows, ok := winsize(f)
	if !ok || cols == 0 || rows == 0 {
		return 0, 0, false
	}
	return cols, rows, true
}

// isTerminal reports whether a file is a terminal.
//
// The question is asked of the kernel rather than of the file's mode, because
// /dev/null is a character device too. A mode check alone says yes to it, and
// `tuiffects > /dev/null` then spends twelve seconds animating into nothing
// instead of copying its input and stopping.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	_, _, ok := winsize(f)
	return ok
}
