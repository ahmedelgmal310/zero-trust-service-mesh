// Command tokengen generates a throwaway JWT signing key, its public JWKS and a
// set of valid and invalid end-user tokens for the e2e attack suite.
//
//	go run ./cmd/tokengen -out /tmp/jwt
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/tokens"
)

func main() {
	out := flag.String("out", "jwt", "output directory")
	issuer := flag.String("issuer", "https://auth.zt-shop.example", "token issuer")
	audience := flag.String("audience", "zt-shop", "token audience")
	flag.Parse()

	set, err := tokens.Generate(*issuer, *audience, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "tokengen:", err)
		os.Exit(1)
	}
	files, err := set.Write(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tokengen:", err)
		os.Exit(1)
	}
	for _, f := range files {
		fmt.Println(f)
	}
}
