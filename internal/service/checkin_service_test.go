package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"entradasya/ms-checkin/internal/model"
	"entradasya/ms-checkin/internal/repository"

	"github.com/google/uuid"
)

type mockTicketsClient struct {
	validateFunc func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error)
}

func (m *mockTicketsClient) ValidateTicket(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
	if m.validateFunc != nil {
		return m.validateFunc(ctx, req, authToken)
	}
	return nil, 0, nil
}

func TestCheckinService_ProcessScan_Success(t *testing.T) {
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New().String()
	ticketCode := "EY-ROCK-123456"
	ticketID := uuid.New().String()
	porterID := uuid.New()
	porterName := "Portero Juan"

	client := &mockTicketsClient{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:        true,
				TicketID:     ticketID,
				EventID:      eventID,
				SectorName:   "VIP",
				CustomerName: "Carlos Gomez",
				Status:       "ISSUED",
			}, http.StatusOK, nil
		},
	}

	svc := NewCheckinService(repo, client)

	resp, status, err := svc.ProcessScan(context.Background(), model.ScanRequest{
		TicketCode: ticketCode,
		EventID:    eventID,
	}, porterID, porterName, "Bearer token")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
	if resp.Result != model.StatusAccepted {
		t.Fatalf("expected result ACCEPTED, got %s", resp.Result)
	}
	if resp.CustomerName != "Carlos Gomez" || resp.SectorName != "VIP" {
		t.Fatalf("unexpected customer/sector: %s / %s", resp.CustomerName, resp.SectorName)
	}
	if resp.CheckedInAt == nil {
		t.Fatalf("expected non-nil checked_in_at")
	}

	// Verify in repository
	accepted, err := repo.FindAcceptedByTicketCode(context.Background(), ticketCode)
	if err != nil || accepted == nil {
		t.Fatalf("expected accepted record in repo, got err=%v, rec=%v", err, accepted)
	}
	if accepted.PorterName != porterName {
		t.Fatalf("expected porterName %s, got %s", porterName, accepted.PorterName)
	}
}

func TestCheckinService_ProcessScan_AlreadyUsed(t *testing.T) {
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New()
	ticketCode := "EY-ROCK-USED12"
	ticketID := uuid.New()
	firstPorterID := uuid.New()
	firstPorterName := "Marcos Puerta 1"

	firstCheckinTime := time.Now().Add(-1 * time.Hour).UTC()

	// Seed existing accepted ticket
	_ = repo.Create(context.Background(), &model.Checkin{
		ID:           uuid.New(),
		TicketCode:   ticketCode,
		TicketID:     &ticketID,
		EventID:      eventID,
		SectorName:   "General",
		CustomerName: "Ana Lopez",
		PorterID:     firstPorterID,
		PorterName:   firstPorterName,
		Status:       model.StatusAccepted,
		CheckedInAt:  firstCheckinTime,
	})

	client := &mockTicketsClient{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:        true,
				TicketID:     ticketID.String(),
				EventID:      eventID.String(),
				SectorName:   "General",
				CustomerName: "Ana Lopez",
				Status:       "ISSUED",
			}, http.StatusOK, nil
		},
	}

	svc := NewCheckinService(repo, client)

	secondPorterID := uuid.New()
	resp, status, err := svc.ProcessScan(context.Background(), model.ScanRequest{
		TicketCode: ticketCode,
		EventID:    eventID.String(),
	}, secondPorterID, "Portero Pedro", "Bearer token")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusConflict {
		t.Fatalf("expected status 409 Conflict, got %d", status)
	}
	if resp.Result != model.StatusRejected || resp.Reason != model.ReasonAlreadyUsed {
		t.Fatalf("expected REJECTED ALREADY_USED, got %s %s", resp.Result, resp.Reason)
	}
	if resp.FirstUsedAt == nil || *resp.FirstUsedAt != firstCheckinTime.Format(time.RFC3339) {
		t.Fatalf("expected first_used_at %s, got %v", firstCheckinTime.Format(time.RFC3339), resp.FirstUsedAt)
	}
}

func TestCheckinService_ProcessScan_EventMismatch(t *testing.T) {
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New().String()
	ticketCode := "EY-ROCK-WRONG"

	client := &mockTicketsClient{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:   false,
				Reason:  model.ReasonEventMismatch,
				Message: "La entrada no pertenece al evento consultado",
			}, http.StatusBadRequest, nil
		},
	}

	svc := NewCheckinService(repo, client)

	resp, status, err := svc.ProcessScan(context.Background(), model.ScanRequest{
		TicketCode: ticketCode,
		EventID:    eventID,
	}, uuid.New(), "Portero Pedro", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", status)
	}
	if resp.Result != model.StatusRejected || resp.Reason != model.ReasonEventMismatch {
		t.Fatalf("expected REJECTED EVENT_MISMATCH, got %s %s", resp.Result, resp.Reason)
	}
}

func TestCheckinService_ProcessScan_InvalidCode(t *testing.T) {
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New().String()
	ticketCode := "NON-EXISTENT"

	client := &mockTicketsClient{
		validateFunc: func(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
			return &model.ValidateTicketResponse{
				Valid:   false,
				Reason:  model.ReasonInvalidCode,
				Message: "El código de boleto no existe en la base de datos",
			}, http.StatusNotFound, nil
		},
	}

	svc := NewCheckinService(repo, client)

	resp, status, err := svc.ProcessScan(context.Background(), model.ScanRequest{
		TicketCode: ticketCode,
		EventID:    eventID,
	}, uuid.New(), "Portero Pedro", "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusNotFound {
		t.Fatalf("expected status 404 Not Found, got %d", status)
	}
	if resp.Result != model.StatusRejected || resp.Reason != model.ReasonInvalidCode {
		t.Fatalf("expected REJECTED INVALID_CODE, got %s %s", resp.Result, resp.Reason)
	}
}

func TestCheckinService_GetStats_And_History(t *testing.T) {
	repo := repository.NewMockCheckinRepository()
	eventID := uuid.New()
	porterID := uuid.New()

	// Seed 2 VIP and 3 General
	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		_ = repo.Create(context.Background(), &model.Checkin{
			TicketCode:   uuid.New().String(),
			EventID:      eventID,
			SectorName:   "VIP",
			CustomerName: "VIP User",
			PorterID:     porterID,
			PorterName:   "Porter 1",
			Status:       model.StatusAccepted,
			CheckedInAt:  now.Add(time.Duration(i) * time.Minute),
		})
	}
	for i := 0; i < 3; i++ {
		_ = repo.Create(context.Background(), &model.Checkin{
			TicketCode:   uuid.New().String(),
			EventID:      eventID,
			SectorName:   "General",
			CustomerName: "General User",
			PorterID:     porterID,
			PorterName:   "Porter 1",
			Status:       model.StatusAccepted,
			CheckedInAt:  now.Add(time.Duration(i+2) * time.Minute),
		})
	}

	svc := NewCheckinService(repo, nil)

	// Test GetStats
	stats, err := svc.GetStats(context.Background(), eventID.String())
	if err != nil {
		t.Fatalf("unexpected error in GetStats: %v", err)
	}
	if stats.TotalCheckedIn != 5 {
		t.Fatalf("expected 5 total checked in, got %d", stats.TotalCheckedIn)
	}
	if len(stats.BySector) != 2 {
		t.Fatalf("expected 2 sectors, got %d", len(stats.BySector))
	}

	// Test GetHistory
	history, err := svc.GetHistory(context.Background(), eventID.String(), 10)
	if err != nil {
		t.Fatalf("unexpected error in GetHistory: %v", err)
	}
	if len(history) != 5 {
		t.Fatalf("expected 5 history items, got %d", len(history))
	}

	// Test invalid event ID
	_, err = svc.GetStats(context.Background(), "invalid-uuid")
	if err != ErrInvalidEventID {
		t.Fatalf("expected ErrInvalidEventID, got %v", err)
	}
}
