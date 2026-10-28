// Command payments captures payments for orders.
package main

import (
	"os"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/payments"
	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
)

func main() {
	os.Exit(platform.Run("payments", payments.New()))
}
