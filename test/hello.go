//go:build ignore

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var toStderr bool
	flag.BoolVar(&toStderr, "stderr", false, "print message to stderr")
	flag.Parse()

	if toStderr {
		fmt.Fprintf(os.Stderr, "hello\n")
	} else {
		fmt.Printf("hello\n")
	}
}
