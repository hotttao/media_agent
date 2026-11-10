package contract

import (
	"errors"
	"strings"
)

func ValidateSegments(values ...string) error {
	for _, v := range values {
		if v == "" || v == "." || v == ".." || strings.ContainsAny(v, "/\\%?#\x00\r\n") {
			return errors.New("invalid path identifier")
		}
	}
	return nil
}
