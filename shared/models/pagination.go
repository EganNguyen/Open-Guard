package models

// PaginatedResponse is the canonical container for list collections
type PaginatedResponse[T any] struct {
	Data []T `json:"data"`
	Meta struct {
		Page       int    `json:"page"`
		PerPage    int    `json:"per_page"`
		Total      int    `json:"total"`
		TotalPages int    `json:"total_pages"`
		NextCursor string `json:"next_cursor,omitempty"`
	} `json:"meta"`
}
