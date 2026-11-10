package contract

import "fmt"

type Error struct{ StatusCode int }

func (e *Error) Error() string { return fmt.Sprintf("platform request failed (%d)", e.StatusCode) }

type Token struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}
