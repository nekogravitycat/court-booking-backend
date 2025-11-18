package response

// PageResponse is the standard wrapper for list endpoints.
type PageResponse[T any] struct {
	Items    []T `json:"items"`
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
	Total    int `json:"total"`
}

// NewPageResponse is a helper to quickly create a response
func NewPageResponse[T any](items []T, page, pageSize, total int) PageResponse[T] {
	// Handle empty slice to avoid JSON outputting null
	if items == nil {
		items = make([]T, 0)
	}

	return PageResponse[T]{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}
}
