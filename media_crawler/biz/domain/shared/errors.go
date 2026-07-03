package shared

import "errors"

// domain 层共享错误（不带基础设施依赖）。
var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrConflict        = errors.New("conflict")
)
