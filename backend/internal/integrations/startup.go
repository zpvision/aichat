package integrations

import (
	"context"
	"fmt"
)

// ValidateSecrets verifies every saved ciphertext, including disabled endpoints.
func (s *Service) ValidateSecrets(ctx context.Context) error {
	return s.Store.validateSecrets(ctx, s.Config.Key)
}

func (s *Store) validateSecrets(ctx context.Context, key []byte) error {
	rows, e := s.DB.Query(ctx, "SELECT organization_id,id,secret_cipher FROM webhook_endpoints")
	if e != nil {
		return fmt.Errorf("webhook secret validation unavailable")
	}
	defer rows.Close()
	for rows.Next() {
		var org, id string
		var b []byte
		if e = rows.Scan(&org, &id, &b); e != nil {
			return fmt.Errorf("webhook secret validation failed")
		}
		if _, e = open(key, org, id, b); e != nil {
			return fmt.Errorf("WEBHOOK_ENCRYPTION_KEY cannot decrypt stored secrets")
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("webhook secret validation unavailable")
	}
	return nil
}
