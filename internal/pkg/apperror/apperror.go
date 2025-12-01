package apperror

// AppError is a custom error type that includes an HTTP status code and an optional internal error code.
type AppError struct {
	Code    int    // HTTP Status Code (e.g., 400, 404)
	Message string // User-facing error message
	Err     error  // The underlying error, if any (not exposed to user)
}

func (e *AppError) Error() string {
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new AppError with a status code and message.
func New(code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// Wrap creates a new AppError wrapping an existing error.
func Wrap(err error, code int, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}
