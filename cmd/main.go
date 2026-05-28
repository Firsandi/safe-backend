package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"safe-backend/internal/handler"
	"safe-backend/internal/middleware"
	"safe-backend/internal/repository"
	"safe-backend/internal/service"
)

func main() {
	// Load .env file manually if it exists
	loadEnv(".env")

	// Set default env vars for local development
	setDefaults()

	// Connection string
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
			os.Getenv("DB_HOST"), os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
		)
	}

	var db *sqlx.DB
	var err error

	// Retry connection
	for i := 0; i < 3; i++ {
		db, err = sqlx.Connect("postgres", dsn)
		if err == nil {
			break
		}
		log.Printf("Waiting for DB... (%d/3) - Error: %v", i+1, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to DB: %v\nDSN: %s", err, dsn)
	}
	defer db.Close()

	// Run migrations
	if err := runMigrations(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Wire dependencies
	userRepo := repository.NewUserRepository(db)
	medicalRepo := repository.NewMedicalProfileRepository(db)
	contactRepo := repository.NewEmergencyContactRepository(db)
	sosRepo := repository.NewSosRepository(db)

	notifierService := service.GetNotificationService()

	authHandler := handler.NewAuthHandler(userRepo)
	profileHandler := handler.NewMedicalProfileHandler(medicalRepo)
	contactHandler := handler.NewEmergencyContactHandler(contactRepo, userRepo, notifierService)
	sosHandler := handler.NewSosHandler(sosRepo, medicalRepo, contactRepo, userRepo, notifierService)

	// Router
	r := gin.Default()

	// CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}))

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "Safe Backend is Running Natively!"})
	})

	api := r.Group("/api")
	{
		api.POST("/register", authHandler.Register)
		api.POST("/login", authHandler.Login)
		api.POST("/auth/google", authHandler.GoogleLogin)
		api.GET("/verify-email", authHandler.VerifyEmail)
		api.POST("/verify-email", authHandler.VerifyEmail)
		api.POST("/resend-verification", authHandler.ResendVerificationEmail)
		api.POST("/cancel-registration", authHandler.CancelRegistration)
		api.POST("/verification-status", authHandler.VerificationStatus)

		// Protected endpoints
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware())
		{
			// Basic Profile
			protected.PUT("/profile", authHandler.UpdateProfile)
			protected.PUT("/profile/fcm", authHandler.UpdateFcmToken)
			protected.PUT("/location", authHandler.UpdateLocation)

			// Medical Profile
			protected.GET("/profile/medical", profileHandler.GetMedicalProfile)
			protected.POST("/profile/medical", profileHandler.UpsertMedicalProfile)

			// Emergency Contacts & User Search
			protected.GET("/users/search", contactHandler.SearchUsers)
			protected.POST("/contacts", contactHandler.AddContact)
			protected.GET("/contacts", contactHandler.ListContacts)
			protected.DELETE("/contacts/:id", contactHandler.DeleteContact)
			protected.GET("/contacts/requests", contactHandler.ListPendingRequests)
			protected.POST("/contacts/requests/:id/accept", contactHandler.AcceptRequest)
			protected.POST("/contacts/requests/:id/reject", contactHandler.RejectRequest)

			// SOS & Tracking
			protected.POST("/sos/trigger", sosHandler.TriggerSos)
			protected.GET("/sos/active", sosHandler.GetActiveSos)
			protected.POST("/sos/:id/resolve", sosHandler.ResolveSos)
			protected.POST("/sos/:id/track", sosHandler.TrackLocation)
			protected.POST("/sos/:id/acknowledge", sosHandler.AcknowledgeSos)
			protected.GET("/sos/:id", sosHandler.GetSosDetail)
			protected.GET("/sos/history/sent", sosHandler.GetSentHistory)
			protected.GET("/sos/history/received", sosHandler.GetReceivedHistory)
		}
	}

	port := os.Getenv("PORT")
	log.Printf("Safe backend is running natively on :%s", port)
	r.Run("0.0.0.0:" + port)
}

func loadEnv(filename string) {
	dir, err := os.Getwd()
	if err != nil {
		return
	}

	for {
		target := filepath.Join(dir, filename)
		file, err := os.Open(target)
		if err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					os.Setenv(key, val)
				}
			}
			log.Printf("Berhasil memuat file konfigurasi dari: %s", target)
			return
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break // Reached filesystem root
		}
		dir = parent
	}
}

func setDefaults() {
	if os.Getenv("DB_HOST") == "" {
		os.Setenv("DB_HOST", "localhost")
	}
	if os.Getenv("DB_PORT") == "" {
		os.Setenv("DB_PORT", "5432")
	}
	if os.Getenv("DB_USER") == "" {
		os.Setenv("DB_USER", "postgres")
	}
	if os.Getenv("DB_PASSWORD") == "" {
		os.Setenv("DB_PASSWORD", "postgres")
	}
	if os.Getenv("DB_NAME") == "" {
		os.Setenv("DB_NAME", "safedb")
	}
	if os.Getenv("JWT_SECRET") == "" {
		os.Setenv("JWT_SECRET", "super_secret_safe_key")
	}
	if os.Getenv("PORT") == "" {
		os.Setenv("PORT", "8080")
	}
}

func runMigrations(db *sqlx.DB) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	var migrationDir string
	var files []os.DirEntry

	for {
		target := filepath.Join(dir, "migration")
		var errRead error
		files, errRead = os.ReadDir(target)
		if errRead == nil {
			migrationDir = target
			break
		}

		targetFallback := filepath.Join(dir, "backend/migration")
		files, errRead = os.ReadDir(targetFallback)
		if errRead == nil {
			migrationDir = targetFallback
			break
		}

		targetFallbackSafe := filepath.Join(dir, "safe-backend/migration")
		files, errRead = os.ReadDir(targetFallbackSafe)
		if errRead == nil {
			migrationDir = targetFallbackSafe
			break
		}

		parent := filepath.Join(dir, "..")
		// Clean the parent path to prevent infinite loops
		parentCleaned := filepath.Clean(parent)
		if parentCleaned == dir {
			return fmt.Errorf("could not find migration, backend/migration, or safe-backend/migration directory in any parent path")
		}
		dir = parentCleaned
	}

	var sqlFiles []string
	for _, f := range files {
		if !f.IsDir() && filepath.Ext(f.Name()) == ".sql" {
			sqlFiles = append(sqlFiles, f.Name())
		}
	}
	sort.Strings(sqlFiles)

	for _, f := range sqlFiles {
		log.Printf("Applying migration: %s", f)
		content, err := os.ReadFile(filepath.Join(migrationDir, f))
		if err != nil {
			return err
		}

		_, err = db.Exec(string(content))
		if err != nil {
			return fmt.Errorf("error in %s: %v", f, err)
		}
	}

	log.Println("All migrations applied successfully!")
	return nil
}
