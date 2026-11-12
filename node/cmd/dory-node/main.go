// Command dory-node es el nodo local-first de Dory.
package main

import (
	"fmt"

	"github.com/migueel15/dory/internal/service"
)

var _ service.Service

const version = "0.0.0-dev"

func main() {
	fmt.Println("dory-node", version)
}
