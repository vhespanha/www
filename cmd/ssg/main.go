// Command ssg generates the static website.
//
// Usage:
//
//	ssg build [-out dir]
//	ssg serve [-out dir] [-addr addr]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"go.vhespanha.dev/www/internal/site"
	"go.vhespanha.dev/www/internal/ssg"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("ssg: ")

	if len(os.Args) < 2 {
		usage()
	}

	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	out := fs.String("out", "dist", "output directory")

	switch os.Args[1] {
	case "build":
		parse(fs)
		if err := build(*out); err != nil {
			log.Fatal(err)
		}
	case "serve":
		addr := fs.String("addr", "localhost:8080", "address to listen on")
		parse(fs)
		log.Printf("serving %s on http://%s", *out, *addr)
		log.Fatal(http.ListenAndServe(*addr, http.FileServer(http.Dir(*out))))
	default:
		usage()
	}
}

func parse(fs *flag.FlagSet) {
	_ = fs.Parse(os.Args[2:])
	if fs.NArg() > 0 {
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ssg build [-out dir]\n       ssg serve [-out dir] [-addr addr]")
	os.Exit(2)
}

func build(out string) error {
	pages, err := site.Pages()
	if err != nil {
		return err
	}
	return ssg.Build(context.Background(), out, pages, site.Assets)
}
