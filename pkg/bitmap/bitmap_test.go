package bitmap

import (
	"testing"
)

func TestBitmap(t *testing.T) {

	size := uint16(BitmapSize)
	b := New(size)
	if b.Size() != size {
		t.Fatalf("wrong size %d", size)
	}

	b.Set(1, 1)
	if b.Get(1) != 1 {
		t.Fatalf("wrong value at bit %d", 1)
	}

	b.Set(1, 0)
	if b.Get(100) != 0 {
		t.Fatalf("wrong value at bit %d", 0)
	}

	b.Set(size, 1)
	if b.Get(size) != 1 {
		t.Fatalf("wrong value at bit %d", size)
	}

	b.Set(size, 0)
	if b.Get(size) != 0 {
		t.Fatalf("wrong value at bit %d", size)
	}

	b.Set(BitmapSize, 1)
	v := b.Get(BitmapSize)
	if v != 1 {
		t.Fatalf("wrong value %d", v)
	}
}
