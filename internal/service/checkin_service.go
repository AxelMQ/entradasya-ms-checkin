package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"entradasya/ms-checkin/internal/client"
	"entradasya/ms-checkin/internal/model"
	"entradasya/ms-checkin/internal/repository"

	"github.com/google/uuid"
)

var (
	ErrInvalidEventID = errors.New("invalid event id format")
)

type CheckinService interface {
	ProcessScan(ctx context.Context, req model.ScanRequest, porterID uuid.UUID, porterName string, authToken string) (*model.ScanResponse, int, error)
	GetStats(ctx context.Context, eventID string) (*model.EventStats, error)
	GetHistory(ctx context.Context, eventID string, limit int) ([]model.ScanHistoryItem, error)
}

type checkinService struct {
	repo          repository.CheckinRepository
	ticketsClient client.TicketsClient
}

func NewCheckinService(repo repository.CheckinRepository, ticketsClient client.TicketsClient) CheckinService {
	return &checkinService{
		repo:          repo,
		ticketsClient: ticketsClient,
	}
}

func (s *checkinService) ProcessScan(ctx context.Context, req model.ScanRequest, porterID uuid.UUID, porterName string, authToken string) (*model.ScanResponse, int, error) {
	eventUUID, err := uuid.Parse(req.EventID)
	if err != nil {
		return &model.ScanResponse{
			Result:  model.StatusRejected,
			Reason:  model.ReasonInvalidCode,
			Message: "El identificador de evento no tiene un formato válido",
		}, http.StatusBadRequest, ErrInvalidEventID
	}

	// 1. Validate ticket against ms-tickets
	valResp, statusCode, err := s.ticketsClient.ValidateTicket(ctx, model.ValidateTicketRequest{
		TicketCode: req.TicketCode,
		EventID:    req.EventID,
	}, authToken)

	if err != nil && (valResp == nil || statusCode == 0) {
		return nil, http.StatusBadGateway, fmt.Errorf("error contacting ticket verification service: %w", err)
	}

	now := time.Now().UTC()

	// 1.1 Ticket does not exist (404)
	if statusCode == http.StatusNotFound || (valResp != nil && !valResp.Valid && valResp.Reason == model.ReasonInvalidCode) {
		reason := model.ReasonInvalidCode
		c := &model.Checkin{
			TicketCode:      req.TicketCode,
			EventID:         eventUUID,
			PorterID:        porterID,
			PorterName:      porterName,
			Status:          model.StatusRejected,
			RejectionReason: &reason,
			CheckedInAt:     now,
		}
		_ = s.repo.Create(ctx, c)

		return &model.ScanResponse{
			Result:  model.StatusRejected,
			Reason:  model.ReasonInvalidCode,
			Message: "El código escaneado no existe",
		}, http.StatusNotFound, nil
	}

	// 1.2 Ticket is for a different event (400)
	if statusCode == http.StatusBadRequest || (valResp != nil && !valResp.Valid && valResp.Reason == model.ReasonEventMismatch) {
		reason := model.ReasonEventMismatch
		c := &model.Checkin{
			TicketCode:      req.TicketCode,
			EventID:         eventUUID,
			PorterID:        porterID,
			PorterName:      porterName,
			Status:          model.StatusRejected,
			RejectionReason: &reason,
			CheckedInAt:     now,
		}
		_ = s.repo.Create(ctx, c)

		return &model.ScanResponse{
			Result:  model.StatusRejected,
			Reason:  model.ReasonEventMismatch,
			Message: "La entrada corresponde a otro evento",
		}, http.StatusBadRequest, nil
	}

	// If any other unexpected error from ms-tickets
	if statusCode != http.StatusOK || valResp == nil || !valResp.Valid {
		msg := "Error en la validación de entrada"
		if valResp != nil && valResp.Message != "" {
			msg = valResp.Message
		}
		return &model.ScanResponse{
			Result:  model.StatusRejected,
			Message: msg,
		}, statusCode, nil
	}

	// 2. Ticket is valid from ms-tickets! Check prior usage in checkin_db
	var parsedTicketID *uuid.UUID
	if valResp.TicketID != "" {
		if tid, err := uuid.Parse(valResp.TicketID); err == nil {
			parsedTicketID = &tid
		}
	}

	existing, err := s.repo.FindAcceptedByTicketCode(ctx, req.TicketCode)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("error verifying prior checkin: %w", err)
	}

	if existing != nil {
		// ALREADY USED!
		firstUsedAt := existing.CheckedInAt.UTC().Format(time.RFC3339)
		reason := model.ReasonAlreadyUsed
		rejectAttempt := &model.Checkin{
			TicketCode:      req.TicketCode,
			TicketID:        parsedTicketID,
			EventID:         eventUUID,
			SectorName:      valResp.SectorName,
			CustomerName:    valResp.CustomerName,
			PorterID:        porterID,
			PorterName:      porterName,
			Status:          model.StatusRejected,
			RejectionReason: &reason,
			CheckedInAt:     now,
		}
		_ = s.repo.Create(ctx, rejectAttempt)

		return &model.ScanResponse{
			Result:      model.StatusRejected,
			Reason:      model.ReasonAlreadyUsed,
			TicketCode:  req.TicketCode,
			FirstUsedAt: &firstUsedAt,
			FirstPorter: existing.PorterName,
			Message:     "Entrada ya fue utilizada anteriormente",
		}, http.StatusConflict, nil
	}

	// 3. Not used yet -> Insert ACCEPTED
	acceptCheckin := &model.Checkin{
		TicketCode:   req.TicketCode,
		TicketID:     parsedTicketID,
		EventID:      eventUUID,
		SectorName:   valResp.SectorName,
		CustomerName: valResp.CustomerName,
		PorterID:     porterID,
		PorterName:   porterName,
		Status:       model.StatusAccepted,
		CheckedInAt:  now,
	}

	err = s.repo.Create(ctx, acceptCheckin)
	if err != nil {
		if errors.Is(err, repository.ErrDuplicateAcceptedTicket) {
			// Race condition: another concurrent process inserted ACCEPTED first
			existingRecord, _ := s.repo.FindAcceptedByTicketCode(ctx, req.TicketCode)
			var firstUsedAt string
			var firstPorter string
			if existingRecord != nil {
				firstUsedAt = existingRecord.CheckedInAt.UTC().Format(time.RFC3339)
				firstPorter = existingRecord.PorterName
			} else {
				firstUsedAt = now.Format(time.RFC3339)
			}

			reason := model.ReasonAlreadyUsed
			rejectAttempt := &model.Checkin{
				TicketCode:      req.TicketCode,
				TicketID:        parsedTicketID,
				EventID:         eventUUID,
				SectorName:      valResp.SectorName,
				CustomerName:    valResp.CustomerName,
				PorterID:        porterID,
				PorterName:      porterName,
				Status:          model.StatusRejected,
				RejectionReason: &reason,
				CheckedInAt:     now,
			}
			_ = s.repo.Create(ctx, rejectAttempt)

			return &model.ScanResponse{
				Result:      model.StatusRejected,
				Reason:      model.ReasonAlreadyUsed,
				TicketCode:  req.TicketCode,
				FirstUsedAt: &firstUsedAt,
				FirstPorter: firstPorter,
				Message:     "Entrada ya fue utilizada anteriormente",
			}, http.StatusConflict, nil
		}

		return nil, http.StatusInternalServerError, fmt.Errorf("error persisting accepted checkin: %w", err)
	}

	checkedInAtStr := acceptCheckin.CheckedInAt.UTC().Format(time.RFC3339)
	return &model.ScanResponse{
		Result:       model.StatusAccepted,
		TicketCode:   req.TicketCode,
		CustomerName: valResp.CustomerName,
		SectorName:   valResp.SectorName,
		CheckedInAt:  &checkedInAtStr,
		Message:      "¡Ingreso permitido!",
	}, http.StatusOK, nil
}

func (s *checkinService) GetStats(ctx context.Context, eventID string) (*model.EventStats, error) {
	if _, err := uuid.Parse(eventID); err != nil {
		return nil, ErrInvalidEventID
	}
	return s.repo.GetStatsByEventID(ctx, eventID)
}

func (s *checkinService) GetHistory(ctx context.Context, eventID string, limit int) ([]model.ScanHistoryItem, error) {
	if _, err := uuid.Parse(eventID); err != nil {
		return nil, ErrInvalidEventID
	}
	return s.repo.GetHistoryByEventID(ctx, eventID, limit)
}
