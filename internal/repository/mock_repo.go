package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"entradasya/ms-checkin/internal/model"

	"github.com/google/uuid"
)

type MockCheckinRepository struct {
	mu          sync.RWMutex
	records     []model.Checkin
	ForceErr    error
	StatsResult *model.EventStats
}

func NewMockCheckinRepository() *MockCheckinRepository {
	return &MockCheckinRepository{
		records: make([]model.Checkin, 0),
	}
}

func (m *MockCheckinRepository) FindAcceptedByTicketCode(ctx context.Context, ticketCode string) (*model.Checkin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ForceErr != nil {
		return nil, m.ForceErr
	}

	for _, r := range m.records {
		if r.TicketCode == ticketCode && r.Status == model.StatusAccepted {
			recordCopy := r
			return &recordCopy, nil
		}
	}

	return nil, nil
}

func (m *MockCheckinRepository) Create(ctx context.Context, c *model.Checkin) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ForceErr != nil {
		return m.ForceErr
	}

	// Emulate unique index idx_unique_accepted_ticket
	if c.Status == model.StatusAccepted {
		for _, r := range m.records {
			if r.TicketCode == c.TicketCode && r.Status == model.StatusAccepted {
				return ErrDuplicateAcceptedTicket
			}
		}
	}

	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	if c.CheckedInAt.IsZero() {
		c.CheckedInAt = time.Now().UTC()
	}

	m.records = append(m.records, *c)
	return nil
}

func (m *MockCheckinRepository) GetStatsByEventID(ctx context.Context, eventID string) (*model.EventStats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ForceErr != nil {
		return nil, m.ForceErr
	}

	if m.StatsResult != nil {
		return m.StatsResult, nil
	}

	stats := &model.EventStats{
		EventID:        eventID,
		TotalCheckedIn: 0,
		BySector:       make([]model.SectorStat, 0),
		LastCheckinAt:  nil,
	}

	sectorCounts := make(map[string]int64)
	var latestTime time.Time

	for _, r := range m.records {
		if r.EventID.String() == eventID && r.Status == model.StatusAccepted {
			stats.TotalCheckedIn++
			sec := r.SectorName
			if sec == "" {
				sec = "General"
			}
			sectorCounts[sec]++
			if r.CheckedInAt.After(latestTime) {
				latestTime = r.CheckedInAt
			}
		}
	}

	if !latestTime.IsZero() {
		formatted := latestTime.UTC().Format(time.RFC3339)
		stats.LastCheckinAt = &formatted
	}

	for sec, count := range sectorCounts {
		stats.BySector = append(stats.BySector, model.SectorStat{
			SectorName: sec,
			CheckedIn:  count,
		})
	}

	// Sort sectors for deterministic tests
	sort.Slice(stats.BySector, func(i, j int) bool {
		return stats.BySector[i].SectorName < stats.BySector[j].SectorName
	})

	return stats, nil
}

func (m *MockCheckinRepository) GetHistoryByEventID(ctx context.Context, eventID string, limit int) ([]model.ScanHistoryItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.ForceErr != nil {
		return nil, m.ForceErr
	}

	if limit <= 0 {
		limit = 20
	}

	eventRecords := make([]model.Checkin, 0)
	for _, r := range m.records {
		if r.EventID.String() == eventID {
			eventRecords = append(eventRecords, r)
		}
	}

	// Sort desc by CheckedInAt
	sort.Slice(eventRecords, func(i, j int) bool {
		return eventRecords[i].CheckedInAt.After(eventRecords[j].CheckedInAt)
	})

	if len(eventRecords) > limit {
		eventRecords = eventRecords[:limit]
	}

	items := make([]model.ScanHistoryItem, 0, len(eventRecords))
	for _, r := range eventRecords {
		var ticketIDStr *string
		if r.TicketID != nil {
			s := r.TicketID.String()
			ticketIDStr = &s
		}

		items = append(items, model.ScanHistoryItem{
			ID:              r.ID.String(),
			TicketCode:      r.TicketCode,
			TicketID:        ticketIDStr,
			EventID:         r.EventID.String(),
			SectorName:      r.SectorName,
			CustomerName:    r.CustomerName,
			PorterID:        r.PorterID.String(),
			PorterName:      r.PorterName,
			Status:          r.Status,
			RejectionReason: r.RejectionReason,
			CheckedInAt:     r.CheckedInAt.UTC().Format(time.RFC3339),
		})
	}

	return items, nil
}
