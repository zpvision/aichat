package knowledge

import (
	"context"
	"site-assistant/internal/billing"
)

// Existing originals are registered without deleting data or pretending an inaccessible file is empty.
func (s *Service) ReconcileStorage(ctx context.Context) error {
	var last error
	after := ""
	for {
		files, e := billing.UnregisteredFiles(ctx, s.Store.DB, after)
		if e != nil {
			return e
		}
		if len(files) == 0 {
			break
		}
		for _, f := range files {
			after = f.ID
			size, e := s.Files.Size(ctx, f.Key)
			if e == nil {
				e = billing.RegisterExisting(ctx, s.Store.DB, f.Org, f.ID, f.Key, size)
			}
			if e != nil {
				last = e
			}
		}
	}
	return last
}
func (s *Service) cleanupStorage(ctx context.Context) error {
	f, e := billing.ClaimCleanupFile(ctx, s.Store.DB)
	if e != nil {
		return e
	}
	if e = s.Files.Delete(ctx, f.Key); e != nil {
		return e
	}
	return billing.ConfirmDeleted(ctx, s.Store.DB, f.Org, f.Key)
}
