package main

import (
	"log"
	"os"

	"github.com/jesseduffield/horcrux/pkg/commands"
)

func main() {
	// I'd use `flaggy` but I like the idea of this repo having no dependencies
	// Unfortunately that means I'm awkwardly making use of the standard flag package
	if len(os.Args) < 2 {
		usage()
	}

	if os.Args[1] == "bind" {
		var dir string
		if len(os.Args) == 2 {
			dir = "."
		} else {
			dir = os.Args[2]
		}
		if err := commands.Bind(dir); err != nil {
			log.Fatal(err)
		}
		return
	}

	if os.Args[len(os.Args)-2] == "split" {
		if len(os.Args) == 2 {
			usage()
		}
		path := os.Args[len(os.Args)-1]
		if err := commands.SplitWithPrompt(path); err != nil {
			log.Fatal(err)
		}
		return
	}

	usage()
}

func usage() {
	log.Fatal("usage: `horcrux bind [<directory>]` | `horcrux [-t] [-n] split <filename>`\n-n: number of horcruxes to make\n-t: number of horcruxes required to resurrect the original file\nexample: horcrux -t 3 -n 5 split diary.txt")
}
