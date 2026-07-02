package store

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// PostgresIdempotencyStore es la implementacion concreta de
// ports.IdempotencyStore sobre PostgreSQL.
// Usa INSERT ... ON CONFLICT ... RETURNING (xmax = 0) para
// detectar si la clave ya existia en una sola operacion atomica.
type PostgresIdempotencyStore struct {
	db *sqlx.DB
}

// NewPostgresIdempotencyStore crea un IdempotencyStore listo para usar.
func NewPostgresIdempotencyStore(db *sqlx.DB) *PostgresIdempotencyStore {
	return &PostgresIdempotencyStore{db: db}
}

// TryInsert intenta insertar la clave. Devuelve true si la inserción
// fue exitosa (primera vez), false si ya existia (duplicado).
//
// Usa el truco de xmax de Postgres: xmax = 0 significa que la fila
// fue insertada por la transacción actual. xmax != 0 significa que
// ya existia previamente.
//
// La clave se hashea con SHA-256 antes de guardarla en la base de
// datos, para evitar almacenar datos sensibles en texto plano.
func (s *PostgresIdempotencyStore) TryInsert(ctx context.Context, key string) (bool, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(key)))

	var inserted bool
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO idempotency_keys (key_hash)
		 VALUES ($1)
		 ON CONFLICT (key_hash) DO UPDATE SET key_hash = EXCLUDED.key_hash
		 RETURNING (xmax = 0)`,
		hash).Scan(&inserted)

	if err != nil {
		return false, fmt.Errorf("insertando clave de idempotencia: %w", err)
	}
	return inserted, nil
}
