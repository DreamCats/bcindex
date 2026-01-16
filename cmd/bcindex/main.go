package main

import (
	"os"

	"github.com/DreamCats/bcindex/internal/bcindex"
)

func main() {
	os.Exit(bcindex.Run(os.Args))
}
