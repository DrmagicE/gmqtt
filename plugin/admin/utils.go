package admin

import (
	"container/list"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrNotFound represents a not found error.
var ErrNotFound = status.Error(codes.NotFound, "not found")

// Indexer provides a index for a ordered list that supports queries in O(1).
// All methods are not concurrency-safe.
type Indexer struct {
	index map[string]*list.Element
	rows  *list.List
}

// NewIndexer is the constructor of Indexer.
func NewIndexer() *Indexer {
	return &Indexer{
		index: make(map[string]*list.Element),
		rows:  list.New(),
	}
}

// Set sets the value for the id.
func (i *Indexer) Set(id string, value interface{}) {
	if e, ok := i.index[id]; ok {
		e.Value = value
	} else {
		elem := i.rows.PushBack(value)
		i.index[id] = elem
	}
}

// Remove removes and returns the value for the given id.
// Return nil if not found.
func (i *Indexer) Remove(id string) *list.Element {
	elem := i.index[id]
	if elem != nil {
		i.rows.Remove(elem)
	}
	delete(i.index, id)
	return elem
}

// GetByID returns the value for the given id.
// Return nil if not found.
// Notice: Any access to the return *list.Element also require the mutex,
// because the Set method can modify the Value for *list.Element when updating the Value for the same id.
// If the caller needs the Value in *list.Element, it must get the Value before the next Set is called.
func (i *Indexer) GetByID(id string) *list.Element {
	return i.index[id]
}

// Iterate iterates at most n elements in the list begin from offset.
// Notice: Any access to the  *list.Element in fn also require the mutex,
// because the Set method can modify the Value for *list.Element when updating the Value for the same id.
// If the caller needs the Value in *list.Element, it must get the Value before the next Set is called.
func (i *Indexer) Iterate(fn func(elem *list.Element), offset, n uint) {
	if i.rows.Len() < int(offset) {
		return
	}
	var j uint
	for e := i.rows.Front(); e != nil; e = e.Next() {
		if j >= offset && j < offset+n {
			fn(e)
		}
		if j == offset+n {
			break
		}
		j++
	}
}

// Len returns the length of list.
func (i *Indexer) Len() int {
	return i.rows.Len()
}

// GetPage gets page and pageSize from request params.
func GetPage(reqPage, reqPageSize uint32) (page, pageSize uint) {
	page = 1
	pageSize = 20
	if reqPage != 0 {
		page = uint(reqPage)
	}
	if reqPageSize != 0 {
		pageSize = uint(reqPageSize)
	}
	return
}

func GetOffsetN(page, pageSize uint) (offset, n uint) {
	offset = (page - 1) * pageSize
	n = pageSize
	return
}

// ErrInvalidArgument is a wrapper function for easier invalid argument error handling.
func ErrInvalidArgument(name string, msg string) error {
	errString := "invalid " + name
	if msg != "" {
		errString = errString + ":" + msg
	}
	return status.Error(codes.InvalidArgument, errString)
}
