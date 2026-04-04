package packets

import (
	"bytes"
	"sync"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// getBuffer retrieves a buffer from the pool.
// The buffer should be returned to the pool using putBuffer when done.
func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// putBuffer returns a buffer to the pool.
func putBuffer(buf *bytes.Buffer) {
	if buf != nil {
		bufferPool.Put(buf)
	}
}
