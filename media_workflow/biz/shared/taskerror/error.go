// Package taskerror classifies a task-function error for the native engine.
// It carries only safe codes, never request payloads, credentials or URLs.
package taskerror

import "errors"

type failure struct {
	code      string
	permanent bool
	cause     error
}

func (e failure) Error() string   { return e.code }
func (e failure) Permanent() bool { return e.permanent }
func (e failure) Unwrap() error   { return e.cause }

func WrapPermanent(code string, cause error) error {
	if cause == nil {
		return nil
	}
	return failure{code: code, permanent: true, cause: cause}
}

func Permanent(code string) error { return failure{code: code, permanent: true} }
func Retryable(code string) error { return failure{code: code} }

func IsPermanent(err error) bool {
	var classified interface{ Permanent() bool }
	return errors.As(err, &classified) && classified.Permanent()
}

func Reason(err error) string {
	var classified failure
	if errors.As(err, &classified) {
		return classified.code
	}
	return "internal_task_failed"
}
