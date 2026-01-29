package main

import (
	"log"
	"os"
)

var debug = os.Getenv("EQLOGUI_DEBUG") == "1"

func debugf(format string, args ...any) {
	if !debug {
		return
	}
	log.Printf(format, args...)
}
