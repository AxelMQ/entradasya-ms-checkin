package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusAccepted = "ACCEPTED"
	StatusRejected = "REJECTED"

	ReasonAlreadyUsed   = "ALREADY_USED"
	ReasonEventMismatch = "EVENT_MISMATCH"
	ReasonInvalidCode   = "INVALID_CODE"
)

// Checkin represents the database entity in checkin_db.checkins
type Checkin struct {
	ID              uuid.UUID  `json:"id"`
	TicketCode      string     `json:"ticket_code"`
	TicketID        *uuid.UUID `json:"ticket_id,omitempty"`
	EventID         uuid.UUID  `json:"event_id"`
	SectorName      string     `json:"sector_name,omitempty"`
	CustomerName    string     `json:"customer_name,omitempty"`
	PorterID        uuid.UUID  `json:"porter_id"`
	PorterName      string     `json:"porter_name"`
	Status          string     `json:"status"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	CheckedInAt     time.Time  `json:"checked_in_at"`
}

// ScanRequest is the DTO received on POST /api/v1/checkin/scan
type ScanRequest struct {
	TicketCode string `json:"ticket_code" binding:"required"`
	EventID    string `json:"event_id" binding:"required"`
}

// ScanResponse is the DTO returned by POST /api/v1/checkin/scan
type ScanResponse struct {
	Result       string  `json:"result"`
	Reason       string  `json:"reason,omitempty"`
	TicketCode   string  `json:"ticket_code,omitempty"`
	CustomerName string  `json:"customer_name,omitempty"`
	SectorName   string  `json:"sector_name,omitempty"`
	CheckedInAt  *string `json:"checked_in_at,omitempty"`
	FirstUsedAt  *string `json:"first_used_at,omitempty"`
	FirstPorter  string  `json:"first_porter,omitempty"`
	Message      string  `json:"message"`
}

// SectorStat represents checked-in count per sector
type SectorStat struct {
	SectorName string `json:"sector_name"`
	CheckedIn  int64  `json:"checked_in"`
}

// EventStats represents the DTO returned by GET /api/v1/checkin/events/:eventId/stats
type EventStats struct {
	EventID        string       `json:"event_id"`
	TotalCheckedIn int64        `json:"total_checked_in"`
	BySector       []SectorStat `json:"by_sector"`
	LastCheckinAt  *string      `json:"last_checkin_at"`
}

// ScanHistoryItem represents an item in the door scan history list
type ScanHistoryItem struct {
	ID              string  `json:"id"`
	TicketCode      string  `json:"ticket_code"`
	TicketID        *string `json:"ticket_id,omitempty"`
	EventID         string  `json:"event_id"`
	SectorName      string  `json:"sector_name,omitempty"`
	CustomerName    string  `json:"customer_name,omitempty"`
	PorterID        string  `json:"porter_id"`
	PorterName      string  `json:"porter_name"`
	Status          string  `json:"status"`
	RejectionReason *string `json:"rejection_reason,omitempty"`
	CheckedInAt     string  `json:"checked_in_at"`
}

// ValidateTicketRequest is sent to ms-tickets POST /api/v1/tickets/validate
type ValidateTicketRequest struct {
	TicketCode string `json:"ticket_code"`
	EventID    string `json:"event_id"`
}

// ValidateTicketResponse is returned by ms-tickets
type ValidateTicketResponse struct {
	Valid            bool   `json:"valid"`
	TicketID         string `json:"ticket_id,omitempty"`
	EventID          string `json:"event_id,omitempty"`
	SectorName       string `json:"sector_name,omitempty"`
	CustomerName     string `json:"customer_name,omitempty"`
	Status           string `json:"status,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Message          string `json:"message,omitempty"`
	ActualEventTitle string `json:"actual_event_title,omitempty"`
}
