package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/TheSnakeFang/rlviz/internal/gallery"
)

func main() {
	output := flag.String("output", "examples/gallery", "gallery output directory")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/gallerygen [-output examples/gallery]")
		os.Exit(2)
	}
	if err := gallery.Generate(*output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
