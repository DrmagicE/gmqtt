package restapi

import (
	"fmt"
	"reflect"
)

// DataProvider interface provides a mechanism to paginate data
type DataProvider interface {
	Models() interface{}
	Count() int
	TotalCount() int
	Pagination() *Pagination
}

// SliceDataProvider implements DataProvider
type SliceDataProvider struct {
	allModels  interface{}
	totalCount int
	count      int
	Pager      *Pagination
}

// Pagination represents information relevant to pagination of data items.
type Pagination struct {
	Page       int
	PageSize   int
	TotalCount int
}

// PageCount returns the page count.
func (p *Pagination) PageCount() int {
	if p.PageSize < 1 {
		if p.TotalCount > 0 {
			return 1
		}
		return 0
	}
	return (p.TotalCount + p.PageSize - 1) / p.PageSize

}

// Offset returns the offset of current page.
func (p *Pagination) Offset() int {
	if p.PageSize < 1 {
		return 0
	}
	return p.Page * p.PageSize
}

// SetModels sets the models into DataProvide.
func (cp *SliceDataProvider) SetModels(models interface{}) {
	if reflect.TypeOf(models).Kind() != reflect.Slice {
		panic(fmt.Sprintf("invalid models type,want slice, but got %s", reflect.TypeOf(models).Kind()))
	}
	cp.allModels = models
	cp.totalCount = reflect.ValueOf(models).Len()

}

// Models set the models of current page into out param.
func (cp *SliceDataProvider) Models(out interface{}) error {
	totalCount := cp.TotalCount()
	rv := reflect.ValueOf(out)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("invalid value,want reflect.Ptr or not nil, got %s", reflect.TypeOf(out).Kind())
	}
	e := rv.Elem()
	if totalCount == 0 {
		return nil
	}
	p := cp.Pagination()
	all := reflect.ValueOf(cp.allModels)
	if cp.Pagination() != nil {
		p.TotalCount = totalCount
		if p.Page+1 > p.PageCount() {
			p.Page = p.PageCount() - 1
		}
		var limit int
		offset := p.Offset()
		if p.PageSize != 0 {
			if offset+p.PageSize > totalCount {
				limit = totalCount
			} else {
				limit = offset + p.PageSize
			}
		} else {
			limit = p.TotalCount
		}
		cp.count = limit - offset
		s := reflect.MakeSlice(e.Type(), cp.count, cp.count)
		reflect.Copy(s, all.Slice(p.Offset(), limit))
		e.Set(all.Slice(p.Offset(), limit))
		return nil
	}
	e.Set(all)
	cp.count = totalCount
	return nil
}

// Count returns the item count of current page.
func (cp *SliceDataProvider) Count() int {
	return cp.count
}

// TotalCount returns the total count of all models
func (cp *SliceDataProvider) TotalCount() int {
	return cp.totalCount
}

// Pagination returns the Pagination struct.
func (cp *SliceDataProvider) Pagination() *Pagination {
	return cp.Pager
}
