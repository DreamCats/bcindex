package main

import (
	"os"

	"bcodingindex/internal/bcindex"
)

func main() {
	os.Exit(bcindex.Run(os.Args))
}
