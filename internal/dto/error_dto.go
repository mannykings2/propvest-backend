package dto

// ErrorResponse is the standard error shape returned by every endpoint
// when something goes wrong.
//
// Using a consistent error shape means the frontend can write one
// error handler that works for every endpoint, instead of special-casing
// each one.
//
// Example response:
//   {
//     "error": "email already registered",
//     "code":  "EMAIL_TAKEN"
//   }
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// ValidationErrorResponse is returned when request binding/validation fails.
// It extends ErrorResponse with a map of field-level errors so the frontend
// can highlight the specific input that failed.
//
// Example response:
//   {
//     "error": "validation failed",
//     "fields": {
//       "email":    "must be a valid email address",
//       "password": "must be at least 8 characters"
//     }
//   }
type ValidationErrorResponse struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields"`
}