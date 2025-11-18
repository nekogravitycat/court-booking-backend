package api

// PageResponse is the standard wrapper for list endpoints.
type PageResponse struct {
	Items    any `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// UpdateUserBody defines fields allowed to be updated via PATCH /users/:id.
// Use pointers to distinguish between "field not sent" and "field sent as false/empty".
type UpdateUserBody struct {
	DisplayName   *string `json:"display_name"`
	IsActive      *bool   `json:"is_active"`
	IsSystemAdmin *bool   `json:"is_system_admin"`
}
