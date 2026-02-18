// Package postgres provides a PostgreSQL implementation of storage interfaces.
// This file contains test helpers only available during testing.
package postgres

import (
	"context"
	"fmt"
)

// TruncateForTest removes all rows from the memories table.
// It is intended for use in tests only. The method is defined in the
// postgres package (not the _test package) so it has access to the
// unexported db field. It is still exported so that the postgres_test
// package can call it.
func (s *MemoryStore) TruncateForTest(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "TRUNCATE TABLE memories RESTART IDENTITY CASCADE")
	if err != nil {
		return fmt.Errorf("postgres: failed to truncate memories: %w", err)
	}
	return nil
}
