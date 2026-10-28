package main

import (
	"log"
	"os"

	"katsironni-hub/internal/config"
	"katsironni-hub/internal/db"
	"katsironni-hub/internal/http"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load Stripe price IDs if available
	if err := config.LoadStripePriceIDs(cfg); err != nil {
		log.Printf("Warning: failed to load Stripe price IDs: %v", err)
	}

	// Open database
	database, err := db.Open(cfg.HubDB)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Create server
	server := http.NewServer(database, cfg)

	// Setup routes
	router := server.SetupRoutes()

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting katsironni-hub on port %s", port)
	if err := router.Run("127.0.0.1:" + port); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}