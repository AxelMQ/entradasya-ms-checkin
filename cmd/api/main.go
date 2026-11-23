package main

import (
	"context"
	"log"
	"time"

	"entradasya/ms-checkin/internal/client"
	"entradasya/ms-checkin/internal/config"
	"entradasya/ms-checkin/internal/database"
	"entradasya/ms-checkin/internal/handler"
	"entradasya/ms-checkin/internal/middleware"
	"entradasya/ms-checkin/internal/repository"
	"entradasya/ms-checkin/internal/service"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.LoadConfig()

	log.Printf("[ms-checkin] Iniciando microservicio de control de acceso en puerto :%s", cfg.Port)

	// Conexión a PostgreSQL (checkin_db)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var dbPinger database.Pinger
	var repo repository.CheckinRepository

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Printf("[ms-checkin] ADVERTENCIA: No se pudo conectar a la base de datos de inmediato: %v", err)
	} else {
		defer db.Close()
		dbPinger = db
		repo = repository.NewPostgresCheckinRepository(db.Pool)
		log.Printf("[ms-checkin] Conexión a PostgreSQL (checkin_db) establecida exitosamente")
	}

	// Fallback mock si la base de datos física no responde durante arranque
	if repo == nil {
		log.Printf("[ms-checkin] ADVERTENCIA: Usando repositorio en memoria como fallback")
		repo = repository.NewMockCheckinRepository()
	}

	// Cliente HTTP hacia ms-tickets
	ticketsClient := client.NewHTTPTicketsClient(cfg.ServicesTicketsURL, cfg.InternalToken)

	// Inicialización de capas
	checkinSvc := service.NewCheckinService(repo, ticketsClient)
	checkinHdl := handler.NewCheckinHandler(checkinSvc)
	healthHdl := handler.NewHealthHandler(dbPinger)

	// Configuración de Gin
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())

	// Probes de salud (Liveness / Readiness)
	r.GET("/health", healthHdl.Live)
	r.GET("/health/live", healthHdl.Live)
	r.GET("/health/ready", healthHdl.Ready)

	// Rutas protegidas de admisión y aforo
	api := r.Group("/api/v1")
	{
		// Escaneo en puerta (requiere rol portero)
		scanGroup := api.Group("/checkin")
		scanGroup.Use(middleware.RequireRole(cfg.JWTSecret, "portero"))
		{
			scanGroup.POST("/scan", checkinHdl.Scan)
		}

		// Aforo en vivo e historial de escaneos (portero u organizador o admin)
		statsGroup := api.Group("/checkin")
		statsGroup.Use(middleware.RequireRole(cfg.JWTSecret, "portero", "organizador", "admin"))
		{
			statsGroup.GET("/events/:eventId/stats", checkinHdl.GetStats)
			statsGroup.GET("/events/:eventId/history", checkinHdl.GetHistory)
		}
	}

	log.Printf("[ms-checkin] Servidor listo y escuchando en :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("[ms-checkin] Error fatal al iniciar servidor HTTP: %v", err)
	}
}
