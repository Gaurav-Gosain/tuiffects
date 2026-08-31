//go:build js && wasm

package main

import (
	"syscall/js"

	booba "github.com/NimbleMarkets/go-booba"

	"github.com/Gaurav-Gosain/tuiffects/demo/internal/showroom"
)

// installBridge registers window.tuiffects_select(name) for the page's
// catalogue. The send happens on a goroutine because a js.FuncOf callback
// must not block, and Program.Send waits for the event loop to take the
// message.
func installBridge(program *booba.Program) {
	js.Global().Set("tuiffects_select", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			name := args[0].String()
			go program.Send(showroom.SelectMsg{Name: name})
		}
		return nil
	}))
}
