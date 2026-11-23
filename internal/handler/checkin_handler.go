package handler

import (
	"errors"
	"net/http"
	"strconv"

	"entradasya/ms-checkin/internal/middleware"
	"entradasya/ms-checkin/internal/model"
	"entradasya/ms-checkin/internal/service"

	"github.com/gin-gonic/gin"
)

type CheckinHandler struct {
	service service.CheckinService
}

func NewCheckinHandler(svc service.CheckinService) *CheckinHandler {
	return &CheckinHandler{service: svc}
}

// Scan handles POST /api/v1/checkin/scan
func (h *CheckinHandler) Scan(c *gin.Context) {
	var req model.ScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "ticket_code and event_id are required fields",
		})
		return
	}

	porterID, porterName, err := middleware.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Unauthorized: invalid user identity in context",
		})
		return
	}

	authToken := c.GetHeader("Authorization")

	resp, statusCode, err := h.service.ProcessScan(c.Request.Context(), req, porterID, porterName, authToken)
	if err != nil && resp == nil {
		c.JSON(statusCode, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(statusCode, resp)
}

// GetStats handles GET /api/v1/checkin/events/:eventId/stats
func (h *CheckinHandler) GetStats(c *gin.Context) {
	eventID := c.Param("eventId")
	if eventID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "eventId is required",
		})
		return
	}

	stats, err := h.service.GetStats(c.Request.Context(), eventID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidEventID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid eventId format, must be UUID",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve event statistics: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// GetHistory handles GET /api/v1/checkin/events/:eventId/history
func (h *CheckinHandler) GetHistory(c *gin.Context) {
	eventID := c.Param("eventId")
	if eventID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "eventId is required",
		})
		return
	}

	limit := 20
	if limitQuery := c.Query("limit"); limitQuery != "" {
		if parsed, err := strconv.Atoi(limitQuery); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	history, err := h.service.GetHistory(c.Request.Context(), eventID, limit)
	if err != nil {
		if errors.Is(err, service.ErrInvalidEventID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid eventId format, must be UUID",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve event scan history: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, history)
}
