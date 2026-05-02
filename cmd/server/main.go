package main

import (
	"fmt"
	"log"

	"github.com/whg517/sqlflow/config"
	"github.com/whg517/sqlflow/internal/api"
	"github.com/whg517/sqlflow/internal/db"
	"github.com/whg517/sqlflow/internal/service"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Initialize database
	database, err := db.Open(cfg.DB.Path)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer database.Close()

	if err := database.Migrate(); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}
	log.Println("database migrated successfully")

	// Initialize services
	authSvc := service.NewAuthService(database.DB, cfg.JWT.Secret, cfg.JWT.Expiry)
	dsSvc := service.NewDatasourceService(database.DB, cfg.EncryptionKey)
	permSvc, err := service.NewPermissionService(database.DB)
	if err != nil {
		log.Fatalf("failed to initialize permission service: %v", err)
	}
	log.Println("permission service initialized")

	historySvc := service.NewQueryHistoryService(database.DB)
	querySvc := service.NewQueryService(database.DB, dsSvc, historySvc, permSvc, cfg.EncryptionKey)
	log.Println("query service initialized")

	// Seed initial admin if users table is empty
	count, err := authSvc.UserCount()
	if err != nil {
		log.Fatalf("failed to count users: %v", err)
	}
	if count == 0 {
		admin, err := authSvc.CreateUser(cfg.Admin.Username, cfg.Admin.Password, "admin")
		if err != nil {
			log.Fatalf("failed to create admin user: %v", err)
		}
		log.Printf("initial admin user created: %s (id=%d)", admin.Username, admin.ID)
	} else {
		log.Println("admin user already exists, skipping seed")
	}

	// Start server
	e := api.NewRouter(authSvc, dsSvc, permSvc, querySvc, historySvc)
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Printf("starting server on %s", addr)
	if err := e.Start(addr); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}
