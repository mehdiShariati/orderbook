package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v4"
)

func (s *Store) InsertEvent(ctx context.Context, typ string, payload any) error {
	t0 := time.Now()
	defer observeDB("insert_event", t0)
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO events (type, payload) VALUES ($1, $2)`, typ, b)
	return err
}

func (s *Store) InsertEventTx(ctx context.Context, tx pgx.Tx, typ string, payload any) error {
	t0 := time.Now()
	defer observeDB("insert_event_tx", t0)
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO events (type, payload) VALUES ($1, $2)`, typ, b)
	return err
}
