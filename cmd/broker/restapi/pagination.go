package restapi

import (
	"net/http"
	"net/url"
	"strconv"
)

//PageAbleObj is used to create response data.
type PageAbleObj struct {
	Models       interface{} `json:"list"`
	Page         int         `json:"page"`
	PageSize     int         `json:"page_size"`
	CurrentCount int         `json:"current_count"`
	TotalCount   int         `json:"total_count"`
	TotalPage    int         `json:"total_page"`
}

// NewHttpPager creates and returns the Pagination by the http GET params.
func NewHttpPager(req *http.Request) *Pagination {
	queryForm, err := url.ParseQuery(req.URL.RawQuery)
	pagination := &Pagination{
		PageSize: 20,
	}
	if err == nil {
		if page := queryForm.Get("page"); page != "" {
			p, err := strconv.Atoi(page)
			if err == nil && p > 1 {
				pagination.Page = p - 1
			}
		}
		if pageSize := queryForm.Get("per-page"); pageSize != "" {
			p, err := strconv.Atoi(pageSize)
			if err == nil && p > 0 {
				pagination.PageSize = p
			}
		}
	}
	return pagination

}
