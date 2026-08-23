package main

import (
	"fmt"
	"os"

	"unpack/internal/unpack"
)

func main() {
	var (
		workingDir string
		err        error
	)

	workingDir, err = os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: get current working directory: %v\n", err)
		os.Exit(1)
	}
	os.Exit(unpack.Run(os.Args[1:], os.Stdout, os.Stderr, workingDir))
}
