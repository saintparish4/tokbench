// Command stubserver runs the fake OpenAI-compatible server from package stub
// on a local port, so tokbench can be exercised end to end for free.
//
// It is a development tool and is not part of the released binary. Numbers
// measured against it describe the flags below, not any real system.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/saintparish4/tokbench/internal/stub"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	first := flag.Duration("first", 200*time.Millisecond, "delay before the first text chunk")
	gap := flag.Duration("gap", 20*time.Millisecond, "delay between later text chunks")
	omitUsage := flag.Bool("omit-usage", false, "leave the usage chunk out of every stream")
	flag.Parse()

	handler := stub.Handler(stub.Options{FirstDelay: *first, Gap: *gap, OmitUsage: *omitUsage})
	log.Printf("stub OpenAI server on http://%s/v1", *addr)
	log.Fatal(http.ListenAndServe(*addr, handler))
}
