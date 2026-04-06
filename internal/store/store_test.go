package store

import (
	"errors"
	"testing"

	"github.com/jackc/pgconn"
)

func TestIsUniqueViolation(t *testing.T) {
	if !IsUniqueViolation(&pgconn.PgError{Code: "23505"}) {
		t.Fatal("expected 23505 to be unique violation")
	}
	if IsUniqueViolation(&pgconn.PgError{Code: "23503"}) {
		t.Fatal("foreign key should not be treated as unique")
	}
	if IsUniqueViolation(errors.New("plain")) {
		t.Fatal("non-pg error should be false")
	}
}
