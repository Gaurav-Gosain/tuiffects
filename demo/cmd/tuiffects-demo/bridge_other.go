//go:build !(js && wasm)

package main

import booba "github.com/NimbleMarkets/go-booba"

// installBridge is a no-op outside the browser: there is no page to select
// an effect from.
func installBridge(*booba.Program) {}
