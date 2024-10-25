package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s seconds\n", os.Args[0])
		os.Exit(2)
	}

	seconds, err := strconv.ParseFloat(os.Args[1], 64)
	if err != nil {
		fmt.Println("Invalid number of seconds:", err)
		return
	}

	time.Sleep(time.Duration(seconds * float64(time.Second)))
	fmt.Printf("T\n")
}
