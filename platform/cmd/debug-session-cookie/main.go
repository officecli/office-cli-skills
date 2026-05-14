package main

import (
	"fmt"
	"os"

	"github.com/officecli/officecli-internal/platform/internal/auth"
)

func main() {
	if len(os.Args) != 3 {
		panic("usage: debug-session-cookie <secret> <session-id>")
	}
	codec := auth.NewSecureCookieCodec(os.Args[1])
	raw, err := codec.Encode(os.Args[2])
	if err != nil {
		panic(err)
	}
	fmt.Print(raw)
}
