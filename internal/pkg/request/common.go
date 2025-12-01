package request

// ByIDRequest is a common struct for endpoints that require an ID path parameter.
type ByIDRequest struct {
	ID string `uri:"id" binding:"required,uuid"`
}

// Validate performs custom validation for ByIDRequest.
func (r *ByIDRequest) Validate() error {
	return nil
}
