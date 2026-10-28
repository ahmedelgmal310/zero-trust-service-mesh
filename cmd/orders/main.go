// Command orders creates orders and charges them through payments.
package main

import (
	"os"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/orders"
	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
)

func main() {
	paymentsURL := platform.Env("PAYMENTS_URL", "http://payments.apps.svc.cluster.local:8080")
	os.Exit(platform.Run("orders", orders.New(paymentsURL, platform.NewClient())))
}
