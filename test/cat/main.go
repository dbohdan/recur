package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
		fmt.Fprintf(os.Stderr, "cat: %v\n", err)
		os.Exit(1)
	}
}
