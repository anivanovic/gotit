package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/anivanovic/gotit/pkg/bencode"
	"go.uber.org/zap"
)

var file = flag.String("file", "", "Path to bencode file")

var log, _ = zap.NewProduction()

func main() {
	flag.Parse()

	if *file == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		log.Fatal("Error reading file", zap.Error(err))
	}

	el, err := bencode.Parse(data)
	if err != nil {
		log.Fatal("Error parsing file", zap.Error(err))
	}

	fmt.Println(el.String())
}
