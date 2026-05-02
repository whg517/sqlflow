package main

import (
	"fmt"
	"log"

	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/api"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	e := api.NewRouter()

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("starting server on %s", addr)
	if err := e.Start(addr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
