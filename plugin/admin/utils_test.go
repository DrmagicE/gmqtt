package admin

import (
	"container/list"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIndexer(t *testing.T) {
	a := assert.New(t)
	i := NewIndexer()
	for j := 0; j < 100; j++ {
		i.Set(strconv.Itoa(j), j)
		a.EqualValues(j, i.GetByID(strconv.Itoa(j)).Value)
	}
	a.EqualValues(100, i.Len())

	var jj int
	i.Iterate(func(elem *list.Element) {
		v := elem.Value.(int)
		a.Equal(jj, v)
		jj++
	}, 0, uint(i.Len()))

	e := i.Remove("5")
	a.Equal(5, e.Value.(int))

	var rs []int
	i.Iterate(func(elem *list.Element) {
		rs = append(rs, elem.Value.(int))
	}, 4, 2)
	// 5 is removed
	a.Equal([]int{4, 6}, rs)

}
