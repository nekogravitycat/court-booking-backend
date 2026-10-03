package request

// ByIDRequest is a common struct for endpoints that require an ID path parameter.
type ByIDRequest struct {
	ID string `uri:"id" binding:"required,uuid"`
}
