package entitlement

import (
	"katsironni-hub/internal/db"
)

type EntitlementService struct {
	db *db.DB
}

func NewEntitlementService(database *db.DB) *EntitlementService {
	return &EntitlementService{db: database}
}

func (s *EntitlementService) GetUserEntitlements(userID string) ([]Entitlement, error) {
	return GetUserEntitlements(s.db, userID)
}

func (s *EntitlementService) HasActiveEntitlement(userID, serviceKey string) (bool, error) {
	return HasActiveEntitlement(s.db, userID, serviceKey)
}

func (s *EntitlementService) GetActiveServices(userID string) ([]string, error) {
	return GetActiveServices(s.db, userID)
}

func (s *EntitlementService) CreateOrUpdateEntitlement(e *Entitlement) error {
	return CreateOrUpdateEntitlement(s.db, e)
}

func (s *EntitlementService) CancelEntitlement(userID, serviceKey, source string) error {
	return CancelEntitlement(s.db, userID, serviceKey, source)
}

func (s *EntitlementService) ExpireOldEntitlements() error {
	return ExpireOldEntitlements(s.db)
}