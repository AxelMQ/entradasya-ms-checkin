package middleware

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		correlationID := c.GetHeader("X-Correlation-ID")
		if correlationID == "" {
			correlationID = uuid.New().String()
			c.Header("X-Correlation-ID", correlationID)
		}

		start := time.Now()
		c.Next()
		latency := time.Since(start)

		log.Printf("[ms-checkin] | %3d | %12v | %s | %-7s %s | correlation_id=%s",
			c.Writer.Status(),
			latency,
			c.ClientIP(),
			c.Request.Method,
			c.Request.URL.Path,
			correlationID,
		)
	}
}
