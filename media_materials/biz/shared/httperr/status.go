package httperr

import (
	"errors"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// Status maps repository and service errors to the stable HTTP contract.
// Callers should return a generic message for 404/409/500 so database and
// authorization details do not escape the service boundary.
func Status(err error, fallback int) int {
	if err == nil {
		return fallback
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 404
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return 409
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") || strings.Contains(message, "not accessible") || strings.Contains(message, "only owner") {
		return 404
	}
	return fallback
}

func Write(c *app.RequestContext, err error, fallback int, resource string) {
	status := Status(err, fallback)
	switch status {
	case 404:
		c.String(status, resource+" not found")
	case 409:
		c.String(status, resource+" conflict")
	case 500:
		c.String(status, "internal server error")
	default:
		c.String(status, err.Error())
	}
}
