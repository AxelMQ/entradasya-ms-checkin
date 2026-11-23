package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"entradasya/ms-checkin/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound                = errors.New("checkin record not found")
	ErrDuplicateAcceptedTicket = errors.New("duplicate accepted ticket")
)

// CheckinRepository defines persistence operations for check-ins
type CheckinRepository interface {
	FindAcceptedByTicketCode(ctx context.Context, ticketCode string) (*model.Checkin, error)
	Create(ctx context.Context, checkin *model.Checkin) error
	GetStatsByEventID(ctx context.Context, eventID string) (*model.EventStats, error)
	GetHistoryByEventID(ctx context.Context, eventID string, limit int) ([]model.ScanHistoryItem, error)
}

type PostgresCheckinRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresCheckinRepository(pool *pgxpool.Pool) *PostgresCheckinRepository {
	return &PostgresCheckinRepository{pool: pool}
}

func (r *PostgresCheckinRepository) FindAcceptedByTicketCode(ctx context.Context, ticketCode string) (*model.Checkin, error) {
	query := `
		SELECT id, ticket_code, ticket_id, event_id, COALESCE(sector_name, ''), COALESCE(customer_name, ''),
		       porter_id, porter_name, status, rejection_reason, checked_in_at
		FROM checkins
		WHERE ticket_code = $1 AND status = 'ACCEPTED'
		LIMIT 1
	`

	var c model.Checkin
	var ticketID *uuid.UUID
	var rejectionReason *string

	err := r.pool.QueryRow(ctx, query, ticketCode).Scan(
		&c.ID,
		&c.TicketCode,
		&ticketID,
		&c.EventID,
		&c.SectorName,
		&c.CustomerName,
		&c.PorterID,
		&c.PorterName,
		&c.Status,
		&rejectionReason,
		&c.CheckedInAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("error querying accepted checkin: %w", err)
	}

	c.TicketID = ticketID
	c.RejectionReason = rejectionReason
	return &c, nil
}

func (r *PostgresCheckinRepository) Create(ctx context.Context, c *model.Checkin) error {
	query := `
		INSERT INTO checkins (
			id, ticket_code, ticket_id, event_id, sector_name, customer_name,
			porter_id, porter_name, status, rejection_reason, checked_in_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`

	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.CheckedInAt.IsZero() {
		c.CheckedInAt = time.Now().UTC()
	}

	_, err := r.pool.Exec(
		ctx,
		query,
		c.ID,
		c.TicketCode,
		c.TicketID,
		c.EventID,
		c.SectorName,
		c.CustomerName,
		c.PorterID,
		c.PorterName,
		c.Status,
		c.RejectionReason,
		c.CheckedInAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrDuplicateAcceptedTicket
		}
		return fmt.Errorf("error inserting checkin: %w", err)
	}

	return nil
}

func (r *PostgresCheckinRepository) GetStatsByEventID(ctx context.Context, eventID string) (*model.EventStats, error) {
	stats := &model.EventStats{
		EventID:        eventID,
		TotalCheckedIn: 0,
		BySector:       make([]model.SectorStat, 0),
		LastCheckinAt:  nil,
	}

	// 1. Total checked in and last checkin
	totalQuery := `
		SELECT COUNT(*), MAX(checked_in_at)
		FROM checkins
		WHERE event_id = $1 AND status = 'ACCEPTED'
	`
	var lastCheckin *time.Time
	err := r.pool.QueryRow(ctx, totalQuery, eventID).Scan(&stats.TotalCheckedIn, &lastCheckin)
	if err != nil {
		return nil, fmt.Errorf("error querying event total stats: %w", err)
	}

	if lastCheckin != nil {
		formatted := lastCheckin.UTC().Format(time.RFC3339)
		stats.LastCheckinAt = &formatted
	}

	// 2. Breakdown by sector
	sectorQuery := `
		SELECT COALESCE(sector_name, 'General'), COUNT(*)
		FROM checkins
		WHERE event_id = $1 AND status = 'ACCEPTED'
		GROUP BY sector_name
		ORDER BY sector_name ASC
	`
	rows, err := r.pool.Query(ctx, sectorQuery, eventID)
	if err != nil {
		return nil, fmt.Errorf("error querying event sector stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var s model.SectorStat
		if err := rows.Scan(&s.SectorName, &s.CheckedIn); err != nil {
			return nil, fmt.Errorf("error scanning sector stat row: %w", err)
		}
		stats.BySector = append(stats.BySector, s)
	}

	return stats, nil
}

func (r *PostgresCheckinRepository) GetHistoryByEventID(ctx context.Context, eventID string, limit int) ([]model.ScanHistoryItem, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := `
		SELECT id, ticket_code, ticket_id, event_id, COALESCE(sector_name, ''), COALESCE(customer_name, ''),
		       porter_id, porter_name, status, rejection_reason, checked_in_at
		FROM checkins
		WHERE event_id = $1
		ORDER BY checked_in_at DESC
		LIMIT $2
	`

	rows, err := r.pool.Query(ctx, query, eventID, limit)
	if err != nil {
		return nil, fmt.Errorf("error querying checkin history: %w", err)
	}
	defer rows.Close()

	items := make([]model.ScanHistoryItem, 0)
	for rows.Next() {
		var id, ticketCode, sectorName, customerName, porterID, porterName, status string
		var ticketID *uuid.UUID
		var eventIDVal uuid.UUID
		var rejectionReason *string
		var checkedInAt time.Time

		err := rows.Scan(
			&id,
			&ticketCode,
			&ticketID,
			&eventIDVal,
			&sectorName,
			&customerName,
			&porterID,
			&porterName,
			&status,
			&rejectionReason,
			&checkedInAt,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning history row: %w", err)
		}

		var ticketIDStr *string
		if ticketID != nil {
			s := ticketID.String()
			ticketIDStr = &s
		}

		items = append(items, model.ScanHistoryItem{
			ID:              id,
			TicketCode:      ticketCode,
			TicketID:        ticketIDStr,
			EventID:         eventIDVal.String(),
			SectorName:      sectorName,
			CustomerName:    customerName,
			PorterID:        porterID,
			PorterName:      porterName,
			Status:          status,
			RejectionReason: rejectionReason,
			CheckedInAt:     checkedInAt.UTC().Format(time.RFC3339),
		})
	}

	return items, nil
}
