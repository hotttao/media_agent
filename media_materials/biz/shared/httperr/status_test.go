package httperr

import (
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "not found", err: fmt.Errorf("load: %w", gorm.ErrRecordNotFound), want: 404},
		{name: "authorization hidden", err: fmt.Errorf("only owner can update material"), want: 404},
		{name: "unique conflict", err: fmt.Errorf("save: %w", &pgconn.PgError{Code: "23505"}), want: 409},
		{name: "validation", err: fmt.Errorf("invalid visibility"), want: 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Status(tt.err, 400); got != tt.want {
				t.Fatalf("Status() = %d, want %d", got, tt.want)
			}
		})
	}
}
