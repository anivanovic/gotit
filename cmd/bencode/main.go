package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/anivanovic/gotit/pkg/bencode"
	"github.com/sirupsen/logrus"
)

var file = flag.String("file", "", "Path to bencode file")

func main() {
	flag.Parse()

	if *file == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	data, err := os.ReadFile(*file)
	if err != nil {
		logrus.Fatal("Error reading file: ", err)
	}

	elements, err := bencode.Parse(string(data))
	if err != nil {
		logrus.Fatal("Error parsing file: ", err)
	}

	for _, el := range elements {
		fmt.Println(el.String())
	}
}
