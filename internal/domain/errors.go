package domain

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeValidation       ErrorCode = "validation_failed"
	CodeNotFound         ErrorCode = "not_found"
	CodeDuplicate        ErrorCode = "duplicate"
	CodeConflict         ErrorCode = "conflict"
	CodeIllegalState     ErrorCode = "illegal_state"
	CodeUnavailable      ErrorCode = "unavailable"
	CodeCorruptStore     ErrorCode = "corrupt_store"
	CodeTimeout          ErrorCode = "timeout"
	CodeCanceled         ErrorCode = "canceled"
	CodeNonConverged     ErrorCode = "non_converged"
	CodeRetryExhausted   ErrorCode = "retry_exhausted"
	CodeFrozenImmutable  ErrorCode = "frozen_immutable"
	CodePreconditionFail ErrorCode = "precondition_failed"
)

type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	EventID int64     `json:"event_id,omitempty"`
	State   string    `json:"state,omitempty"`
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	if e.State != "" {
		return fmt.Sprintf("%s: %s (state=%s)", e.Code, e.Message, e.State)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(code ErrorCode, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

func Errorf(code ErrorCode, format string, args ...any) *AppError {
	return &AppError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func WithEvent(err error, eventID int64) error {
	var app *AppError
	if errors.As(err, &app) {
		cp := *app
		cp.EventID = eventID
		return &cp
	}
	return err
}

func ErrorCodeOf(err error) ErrorCode {
	var app *AppError
	if errors.As(err, &app) {
		return app.Code
	}
	return ""
}

func IsCode(err error, code ErrorCode) bool {
	return ErrorCodeOf(err) == code
}
