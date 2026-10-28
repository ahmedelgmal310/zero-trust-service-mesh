// Command reports serves analytics reports.
package main

import (
	"os"

	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/platform"
	"github.com/ahmedelgmal310/zero-trust-service-mesh/internal/reports"
)

func main() {
	os.Exit(platform.Run("reports", reports.New()))
}
