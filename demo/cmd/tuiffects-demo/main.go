// tuiffects-demo plays the tuiffects catalogue in a terminal, or in a
// browser when compiled to WebAssembly by booba-wasm-build. The same file
// builds both: booba.NewProgram picks the runtime for the target.
//
//	tuiffects-demo [--fps N] [--seed N] [effect]
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"

	booba "github.com/NimbleMarkets/go-booba"

	"github.com/Gaurav-Gosain/tuiffects/demo/internal/showroom"
)

func main() {
	fps := flag.Int("fps", 60, "frames per second")
	seed := flag.Uint64("seed", 0, "random seed; 0 takes a fresh one from the clock for every run")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] [effect]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()

	program := booba.NewProgram(showroom.New(showroom.Options{
		Effect: flag.Arg(0),
		FPS:    *fps,
		Seed:   *seed,
		// A quit in the browser would leave a dead terminal on the page.
		QuitKeys: runtime.GOOS != "js",
	}))
	installBridge(program)
	if _, err := program.Run(); err != nil {
		log.Fatal(err)
	}
}
