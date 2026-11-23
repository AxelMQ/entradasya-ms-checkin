package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"entradasya/ms-checkin/internal/middleware"
	"entradasya/ms-checkin/internal/model"
	"entradasya/ms-checkin/internal/repository"
	"entradasya/ms-checkin/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type mockTicketsClientForHandler struct {
	validateFunc func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error)
}

func (m *mockTicketsClientForHandler) ValidateTicket(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, req, authToken)
	}
	return nil, 0, nil
}

type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(ctx context.Context) error {
	return m.err
}

func makeToken(secret, sub, name, role string) string {
	claims := middleware.CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			Issuer:    "entradasya-auth",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
		},
		Name: name,
		Role: role,
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := t.SignedString([]byte(secret))
	return s
}

func setupTestApp(secret string, repo repository.CheckinRepository, client *mockTicketsClientForHandler, pinger *mockPinger) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	svc := service.NewCheckinService(repo, client)
	checkinHdl := NewCheckinHandler(svc)
	healthHdl := NewHealthHandler(pinger)

	r.GET("/health", healthHdl.Live)
	r.GET("/health/live", healthHdl.Live)
	r.GET("/health/ready", healthHdl.Ready)

	api := r.Group("/api/v1")
	{
		// Escaneo en puerta: solo portero
		scanGroup := api.Group("/checkin")
		scanGroup.Use(middleware.RequireRole(secret, "portero"))
		scanGroup.POST("/scan", checkinHdl.Scan)

		// Estadísticas e historial: portero u organizador
		statsGroup := api.Group("/checkin")
		statsGroup.Use(middleware.RequireRole(secret, "portero", "organizador"))
		statsGroup.GET("/events/:eventId/stats", checkinHdl.GetStats)
		statsGroup.GET("/events/:eventId/history", checkinHdl.GetHistory)
	}

	return r
}

func TestCheckinHandler_Scan_Success(t *testing.T) {
	secret := "test-secret"
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New().String()
	ticketCode := "EY-ROCK-111222"
	ticketID := uuid.New().String()

	client := &mockTicketsClientForHandler{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:        true,
				TicketID:     ticketID,
				EventID:      eventID,
				SectorName:   "Platea",
				CustomerName: "Maria Lopez",
				Status:       "ISSUED",
			}, http.StatusOK, nil
		},
	}

	app := setupTestApp(secret, repo, client, &mockPinger{})
	porterToken := makeToken(secret, uuid.New().String(), "Portero Pedro", "portero")

	body, _ := json.Marshal(model.ScanRequest{
		TicketCode: ticketCode,
		EventID:    eventID,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkin/scan", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+porterToken)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp model.ScanResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Result != "ACCEPTED" || resp.CustomerName != "Maria Lopez" || resp.SectorName != "Platea" {
		t.Fatalf("unexpected scan response: %+v", resp)
	}
}

func TestCheckinHandler_Scan_AlreadyUsed_Conflict(t *testing.T) {
	secret := "test-secret"
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New()
	ticketCode := "EY-ROCK-REPEAT"
	firstUsedTime := time.Now().Add(-30 * time.Minute).UTC()

	// Pre-seed accepted ticket in repo
	_ = repo.Create(context.Background(), &model.Checkin{
		ID:           uuid.New(),
		TicketCode:   ticketCode,
		EventID:      eventID,
		SectorName:   "VIP",
		CustomerName: "Juan Perez",
		PorterID:     uuid.New(),
		PorterName:   "Marcos Puerta 1",
		Status:       model.StatusAccepted,
		CheckedInAt:  firstUsedTime,
	})

	client := &mockTicketsClientForHandler{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:        true,
				TicketID:     uuid.New().String(),
				EventID:      eventID.String(),
				SectorName:   "VIP",
				CustomerName: "Juan Perez",
				Status:       "ISSUED",
			}, http.StatusOK, nil
		},
	}

	app := setupTestApp(secret, repo, client, &mockPinger{})
	porterToken := makeToken(secret, uuid.New().String(), "Portero Segundo", "portero")

	body, _ := json.Marshal(model.ScanRequest{
		TicketCode: ticketCode,
		EventID:    eventID.String(),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkin/scan", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+porterToken)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp model.ScanResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Result != "REJECTED" || resp.Reason != "ALREADY_USED" {
		t.Fatalf("expected REJECTED ALREADY_USED, got %+v", resp)
	}
	if resp.FirstUsedAt == nil || *resp.FirstUsedAt != firstUsedTime.Format(time.RFC3339) {
		t.Fatalf("expected first_used_at %s, got %v", firstUsedTime.Format(time.RFC3339), resp.FirstUsedAt)
	}
}

func TestCheckinHandler_Scan_EventMismatch(t *testing.T) {
	secret := "test-secret"
	repo := repository.NewMockCheckinRepository()

	client := &mockTicketsClientForHandler{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:   false,
				Reason:  "EVENT_MISMATCH",
				Message: "La entrada no pertenece al evento consultado",
			}, http.StatusBadRequest, nil
		},
	}

	app := setupTestApp(secret, repo, client, &mockPinger{})
	porterToken := makeToken(secret, uuid.New().String(), "Portero Pedro", "portero")

	body, _ := json.Marshal(model.ScanRequest{
		TicketCode: "EY-JAZZ-999999",
		EventID:    uuid.New().String(),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkin/scan", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+porterToken)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp model.ScanResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Result != "REJECTED" || resp.Reason != "EVENT_MISMATCH" {
		t.Fatalf("expected REJECTED EVENT_MISMATCH, got %+v", resp)
	}
}

func TestCheckinHandler_Scan_InvalidCode(t *testing.T) {
	secret := "test-secret"
	repo := repository.NewMockCheckinRepository()

	client := &mockTicketsClientForHandler{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:   false,
				Reason:  "INVALID_CODE",
				Message: "El código de boleto no existe en la base de datos",
			}, http.StatusNotFound, nil
		},
	}

	app := setupTestApp(secret, repo, client, &mockPinger{})
	porterToken := makeToken(secret, uuid.New().String(), "Portero Pedro", "portero")

	body, _ := json.Marshal(model.ScanRequest{
		TicketCode: "UNKNOWN-CODE",
		EventID:    uuid.New().String(),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checkin/scan", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+porterToken)
	w := httptest.NewRecorder()

	app.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp model.ScanResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Result != "REJECTED" || resp.Reason != "INVALID_CODE" {
		t.Fatalf("expected REJECTED INVALID_CODE, got %+v", resp)
	}
}

func TestCheckinHandler_Scan_AuthErrors(t *testing.T) {
	secret := "test-secret"
	repo := repository.NewMockCheckinRepository()
	app := setupTestApp(secret, repo, &mockTicketsClientForHandler{}, &mockPinger{})

	body, _ := json.Marshal(model.ScanRequest{
		TicketCode: "EY-ROCK-123456",
		EventID:    uuid.New().String(),
	})

	// 1. Missing Token -> 401
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/checkin/scan", bytes.NewBuffer(body))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	app.ServeHTTP(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for missing token, got %d", w1.Code)
	}

	// 2. Forbidden Role (cliente) -> 403
	clientToken := makeToken(secret, uuid.New().String(), "Cliente Común", "cliente")
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/checkin/scan", bytes.NewBuffer(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+clientToken)
	w2 := httptest.NewRecorder()
	app.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for client role, got %d", w2.Code)
	}
}

func TestCheckinHandler_Stats_And_History(t *testing.T) {
	secret := "test-secret"
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New()
	porterID := uuid.New()

	// Seed 3 accepted entries
	now := time.Now().UTC()
	_ = repo.Create(context.Background(), &model.Checkin{
		TicketCode:   "T1",
		EventID:      eventID,
		SectorName:   "VIP",
		CustomerName: "User 1",
		PorterID:     porterID,
		PorterName:   "Porter 1",
		Status:       model.StatusAccepted,
		CheckedInAt:  now,
	})
	_ = repo.Create(context.Background(), &model.Checkin{
		TicketCode:   "T2",
		EventID:      eventID,
		SectorName:   "General",
		CustomerName: "User 2",
		PorterID:     porterID,
		PorterName:   "Porter 1",
		Status:       model.StatusAccepted,
		CheckedInAt:  now.Add(1 * time.Minute),
	})

	app := setupTestApp(secret, repo, &mockTicketsClientForHandler{}, &mockPinger{})
	organizerToken := makeToken(secret, uuid.New().String(), "Organizador Laura", "organizador")

	// 1. GET /api/v1/checkin/events/:eventId/stats
	reqStats := httptest.NewRequest(http.MethodGet, "/api/v1/checkin/events/"+eventID.String()+"/stats", nil)
	reqStats.Header.Set("Authorization", "Bearer "+organizerToken)
	wStats := httptest.NewRecorder()
	app.ServeHTTP(wStats, reqStats)

	if wStats.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for stats, got %d. Body: %s", wStats.Code, wStats.Body.String())
	}

	var stats model.EventStats
	_ = json.Unmarshal(wStats.Body.Bytes(), &stats)
	if stats.TotalCheckedIn != 2 || len(stats.BySector) != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// 2. GET /api/v1/checkin/events/:eventId/history
	reqHist := httptest.NewRequest(http.MethodGet, "/api/v1/checkin/events/"+eventID.String()+"/history?limit=10", nil)
	reqHist.Header.Set("Authorization", "Bearer "+organizerToken)
	wHist := httptest.NewRecorder()
	app.ServeHTTP(wHist, reqHist)

	if wHist.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for history, got %d. Body: %s", wHist.Code, wHist.Body.String())
	}

	var history []model.ScanHistoryItem
	_ = json.Unmarshal(wHist.Body.Bytes(), &history)
	if len(history) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(history))
	}
}

func TestCheckinHandler_HealthChecks(t *testing.T) {
	secret := "test-secret"
	repo := repository.NewMockCheckinRepository()

	// 1. Live probe
	appLive := setupTestApp(secret, repo, nil, &mockPinger{})
	reqLive := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	wLive := httptest.NewRecorder()
	appLive.ServeHTTP(wLive, reqLive)

	if wLive.Code != http.StatusOK {
		t.Fatalf("expected 200 UP for /health/live, got %d", wLive.Code)
	}

	// 2. Ready probe with healthy DB
	reqReadyOK := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	wReadyOK := httptest.NewRecorder()
	appLive.ServeHTTP(wReadyOK, reqReadyOK)

	if wReadyOK.Code != http.StatusOK {
		t.Fatalf("expected 200 READY for /health/ready, got %d", wReadyOK.Code)
	}

	// 3. Ready probe with failing DB
	appDown := setupTestApp(secret, repo, nil, &mockPinger{err: errors.New("connection refused")})
	reqReadyDown := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	wReadyDown := httptest.NewRecorder()
	appDown.ServeHTTP(wReadyDown, reqReadyDown)

	if wReadyDown.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", wReadyDown.Code)
	}
}
