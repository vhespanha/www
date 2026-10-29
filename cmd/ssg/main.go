// Command ssg generates the static website.
//
// Usage:
//
//	ssg build [-out dir]
//	ssg serve [-out dir] [-addr addr]
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
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
		_ = fs.Parse(os.Args[2:])
		if err := build(*out); err != nil {
			log.Fatal(err)
		}
	case "serve":
		addr := fs.String("addr", "localhost:8080", "address to listen on")
		_ = fs.Parse(os.Args[2:])
		log.Printf("serving %s on http://%s", *out, *addr)
		log.Fatal(http.ListenAndServe(*addr, http.FileServer(http.Dir(*out))))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: ssg build [-out dir]\n       ssg serve [-out dir] [-addr addr]")
	os.Exit(2)
}

func build(out string) error {
	// TODO: render the site package into out using the ssg package.
	return os.MkdirAll(out, 0o755)
}
