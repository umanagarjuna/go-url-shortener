package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/umanagarjuna/go-url-shortener/api/proto/url/v1"
	"github.com/umanagarjuna/go-url-shortener/internal/url/cache"
	"github.com/umanagarjuna/go-url-shortener/internal/url/config"
	"github.com/umanagarjuna/go-url-shortener/internal/url/events"
	"github.com/umanagarjuna/go-url-shortener/internal/url/handler"
	"github.com/umanagarjuna/go-url-shortener/internal/url/metrics"
	"github.com/umanagarjuna/go-url-shortener/internal/url/repository"
	"github.com/umanagarjuna/go-url-shortener/internal/url/service"
	"github.com/umanagarjuna/go-url-shortener/pkg/shortcode"
	"github.com/umanagarjuna/go-url-shortener/pkg/validator"
)

func main() {
	// Initialize logger
	logger, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Initialize database
	db, err := initDB(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}
	defer db.Close()

	// Initialize Redis
	redisClient := initRedis(cfg.Redis)
	defer redisClient.Close()

	// Initialize Kafka publisher
	publisher, err := events.NewEventPublisher(cfg.Kafka.Brokers)
	if err != nil {
		logger.Fatal("Failed to initialize event publisher", zap.Error(err))
	}
	defer publisher.Close()

	// Initialize dependencies
	repo := repository.NewPostgresRepository(db)
	cacheLayer := cache.NewRedisCache(redisClient)
	generator := shortcode.NewBase62GeneratorWithLength(10)
	urlValidator := validator.NewDefaultValidator()

	// Initialize metrics
	metricsCollector := metrics.NewInMemoryMetrics()

	// Initialize service
	urlService := service.NewURLService(
		repo,
		cacheLayer,
		generator,
		urlValidator,
		publisher,
		logger,
		metricsCollector, // NEW
		service.Config{
			BaseURL: cfg.Service.BaseURL,
		},
	)

	// Start servers
	errChan := make(chan error, 2)

	// Start HTTP server
	go func() {
		httpHandler := handler.NewHTTPHandler(urlService, logger)
		router := setupHTTPRouter(httpHandler)

		srv := &http.Server{
			Addr:    cfg.Server.HTTPPort,
			Handler: router,
		}

		logger.Info("Starting HTTP server", zap.String("port", cfg.Server.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	// Start gRPC server
	go func() {
		grpcHandler := handler.NewGRPCHandler(urlService)

		lis, err := net.Listen("tcp", cfg.Server.GRPCPort)
		if err != nil {
			errChan <- fmt.Errorf("failed to listen: %w", err)
			return
		}

		grpcServer := grpc.NewServer()
		pb.RegisterURLServiceServer(grpcServer, grpcHandler)

		logger.Info("Starting gRPC server", zap.String("port", cfg.Server.GRPCPort))
		if err := grpcServer.Serve(lis); err != nil {
			errChan <- fmt.Errorf("gRPC server error: %w", err)
		}
	}()

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errChan:
		logger.Fatal("Server error", zap.Error(err))
	case sig := <-sigChan:
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	}

	logger.Info("Server stopped")
}

func initDB(cfg config.DatabaseConfig) (*sqlx.DB, error) {
	db, err := sqlx.Connect("postgres", cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

func initRedis(cfg config.RedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

func setupHTTPRouter(handler *handler.HTTPHandler) *gin.Engine {
	router := gin.Default()

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Register routes
	handler.RegisterRoutes(router)

	return router
}
