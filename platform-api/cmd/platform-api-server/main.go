package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/stdr"
	_ "github.com/lib/pq"
	"github.com/openshift-online/gecko/orlop/pkg/apiserver"
	"github.com/openshift-online/gecko/orlop/pkg/apiserver/storage/postgres"
)

func main() {
	var (
		address      string
		privatePort  int
		publicPort   int
		corsOrigins  string
		enablePublic bool
	)

	flag.StringVar(&address, "address", "0.0.0.0", "address to bind to")
	flag.IntVar(&privatePort, "private-port", 8080, "port for private API")
	flag.IntVar(&publicPort, "public-port", 8081, "port for public API")
	flag.BoolVar(&enablePublic, "enable-public-api", true, "enable public API server")
	flag.StringVar(&corsOrigins, "cors-origins", "*", "comma-separated list of allowed CORS origins")
	flag.Parse()

	logger := stdr.New(nil)

	// Parse CORS origins
	origins := []string{}
	if corsOrigins != "" {
		origins = strings.Split(corsOrigins, ",")
		for i, origin := range origins {
			origins[i] = strings.TrimSpace(origin)
		}
	}

	// Configure storage backend
	var storageFactory apiserver.StorageFactory
	if dbHost := os.Getenv("DB_HOST"); dbHost != "" {
		dbPort := os.Getenv("DB_PORT")
		if dbPort == "" {
			dbPort = "5432"
		}
		dbName := os.Getenv("DB_NAME")
		if dbName == "" {
			dbName = "orlop"
		}
		dbUser := os.Getenv("DB_USER")
		if dbUser == "" {
			dbUser = "orlop"
		}
		dbPassword := os.Getenv("DB_PASSWORD")
		dbSSLMode := os.Getenv("DB_SSLMODE")
		if dbSSLMode == "" {
			dbSSLMode = "disable"
		}

		connStr := fmt.Sprintf("host=%s port=%s dbname=%s user=%s password=%s sslmode=%s",
			dbHost, dbPort, dbName, dbUser, dbPassword, dbSSLMode)

		db, err := sql.Open("postgres", connStr)
		if err != nil {
			log.Fatalf("Failed to open database: %v", err)
		}
		defer db.Close()

		if err := db.Ping(); err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}

		log.Printf("Connected to PostgreSQL at %s:%s/%s", dbHost, dbPort, dbName)

		storageFactory = postgres.NewStorageFactory(postgres.StorageFactoryConfig{
			DB:         db,
			ConnString: connStr,
			Context:    context.Background(),
		})
	}

	// Create server with resource configuration
	opts := apiserver.Options{
		Address: address,
		Private: apiserver.PrivateAPIOptions{
			Port:      privatePort,
			Resources: getPrivateResources(),
			Scheme:    getPrivateScheme(),
		},
		Public: apiserver.PublicAPIOptions{
			Enable:    enablePublic,
			Port:      publicPort,
			Resources: getPublicResources(),
			Scheme:    getPublicScheme(),
		},
		CORSOrigins:    origins,
		StorageFactory: storageFactory,
		Logger:         logger,
	}

	server, err := apiserver.New(opts)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		if err := server.Run(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for signal
	<-sigChan
	log.Println("Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}
