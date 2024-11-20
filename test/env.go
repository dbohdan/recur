package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	envRecurAttempt := os.Getenv("RECUR_ATTEMPT")
	attempt, err := strconv.Atoi(envRecurAttempt)
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(101)
	}

	envRecurMaxAttempts := os.Getenv("RECUR_MAX_ATTEMPTS")
	maxAttempts, err := strconv.Atoi(envRecurMaxAttempts)
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(102)
	}

	if attempt == 3 && maxAttempts == 10 {
		os.Exit(0)
	} else {
		os.Exit(103)
	}
}
