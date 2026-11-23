package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"entradasya/ms-checkin/internal/model"
)

func TestHTTPTicketsClient_ValidateTicket_Success(t *testing.T) {
	expectedTicketID := "1a2b3c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d"
	expectedEventID := "b1a2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tickets/validate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Internal-Token") != "test-token" {
			t.Fatalf("expected X-Internal-Token test-token, got %s", r.Header.Get("X-Internal-Token"))
		}
		if r.Header.Get("Authorization") != "Bearer test-jwt" {
			t.Fatalf("expected Authorization Bearer test-jwt, got %s", r.Header.Get("Authorization"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(model.ValidateTicketResponse{
			Valid:        true,
			TicketID:     expectedTicketID,
			EventID:      expectedEventID,
			SectorName:   "VIP",
			CustomerName: "Juan Perez",
			Status:       "ISSUED",
		})
	}))
	defer server.Close()

	c := NewHTTPTicketsClient(server.URL, "test-token")
	resp, statusCode, err := c.ValidateTicket(context.Background(), model.ValidateTicketRequest{
		TicketCode: "EY-ROCK-8X4N2Q",
		EventID:    expectedEventID,
	}, "Bearer test-jwt")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", statusCode)
	}
	if !resp.Valid || resp.TicketID != expectedTicketID {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHTTPTicketsClient_ValidateTicket_EventMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ValidateTicketResponse{
			Valid:   false,
			Reason:  "EVENT_MISMATCH",
			Message: "La entrada no pertenece al evento consultado",
		})
	}))
	defer server.Close()

	c := NewHTTPTicketsClient(server.URL, "test-token")
	resp, statusCode, err := c.ValidateTicket(context.Background(), model.ValidateTicketRequest{
		TicketCode: "EY-ROCK-8X4N2Q",
		EventID:    "wrong-event",
	}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", statusCode)
	}
	if resp.Valid || resp.Reason != "EVENT_MISMATCH" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHTTPTicketsClient_ValidateTicket_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(model.ValidateTicketResponse{
			Valid:   false,
			Reason:  "INVALID_CODE",
			Message: "El código de boleto no existe en la base de datos",
		})
	}))
	defer server.Close()

	c := NewHTTPTicketsClient(server.URL, "test-token")
	resp, statusCode, err := c.ValidateTicket(context.Background(), model.ValidateTicketRequest{
		TicketCode: "NONEXISTENT",
		EventID:    "b1a2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
	}, "")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", statusCode)
	}
	if resp.Valid || resp.Reason != "INVALID_CODE" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
