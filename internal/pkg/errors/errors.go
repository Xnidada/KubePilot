package errors

import "fmt"

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func New(code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

func Wrap(err error, message string) *AppError {
	return &AppError{
		Code:    500,
		Message: message,
		Err:     err,
	}
}

func WithCode(err error, code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// Common errors
var (
	ErrNotFound       = New(404, "resource not found")
	ErrUnauthorized   = New(401, "unauthorized")
	ErrForbidden      = New(403, "forbidden")
	ErrBadRequest     = New(400, "bad request")
	ErrInternal       = New(500, "internal server error")
	ErrDuplicateEntry = New(409, "duplicate entry")
	ErrInvalidToken   = New(401, "invalid token")
	ErrExpiredToken   = New(401, "token expired")
)
