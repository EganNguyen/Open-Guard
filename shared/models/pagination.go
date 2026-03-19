package models

// PaginatedResponse is the standard list endpoint envelope.
type PaginatedResponse struct {
	Data interface{}    `json:"data"`
	Meta PaginationMeta `json:"meta"`
}

// PaginationMeta holds pagination metadata.
type PaginationMeta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// NewPaginationMeta calculates pagination metadata.
func NewPaginationMeta(page, perPage, total int) PaginationMeta {
	totalPages := total / perPage
	if total%perPage != 0 {
		totalPages++
	}
	return PaginationMeta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}
}
