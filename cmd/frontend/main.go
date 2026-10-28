// Command frontend serves the edge-facing checkout API.
package main

import (
	"os"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/frontend"
	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
)

func main() {
	ordersURL := platform.Env("ORDERS_URL", "http://orders.apps.svc.cluster.local:8080")
	os.Exit(platform.Run("frontend", frontend.New(ordersURL, platform.NewClient())))
}
