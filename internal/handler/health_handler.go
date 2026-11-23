package handler

import (
	"context"
	"net/http"
	"time"

	"entradasya/ms-checkin/internal/database"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	db database.Pinger
}

func NewHealthHandler(db database.Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

func (h *HealthHandler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "UP",
		"service":   "ms-checkin",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "DOWN",
			"service": "ms-checkin",
			"error":   "database connection not initialized",
		})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "DOWN",
			"service": "ms-checkin",
			"error":   "database unreachable: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "READY",
		"service":  "ms-checkin",
		"database": "connected",
	})
}
