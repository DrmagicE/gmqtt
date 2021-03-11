package bitmap

const (
	//BitmapSize 最大支持的大小
	BitmapSize = 65535
	//Unused 比特位标记为0
	Unused = 0
	//Used 比特位标记为1
	Used = 1
)

//Bitmap Bitmap结构体
type Bitmap struct {
	vals []byte
	size uint16
}

//New New BitMap
func New(size uint16) *Bitmap {
	if size == 0 || size >= BitmapSize {
		size = BitmapSize
	} else if remainder := size % 8; remainder != 0 {
		size += 8 - remainder
	}

	return &Bitmap{size: size, vals: make([]byte, size/8+1)}
}

// IsUsed 指定offset位置值为1
func (b *Bitmap) IsUsed(offset uint16) bool {
	if b.getBit(offset) == Used {
		return true
	}
	return false
}

// Used 设置offset位置为1
func (b *Bitmap) Used(offset uint16) bool {
	return b.setBit(offset, Used)
}

// Unused 设置offset位置值为0
func (b *Bitmap) Unused(offset uint16) bool {
	return b.setBit(offset, Unused)
}

//SetBit 内部使用，将offset位置的值设置为value(0/1)
func (b *Bitmap) setBit(offset uint16, value uint8) bool {
	index, pos := offset/8, offset%8

	if b.size < offset {
		return false
	}

	if value == 0 {
		b.vals[index] &^= 0x01 << pos
	} else {
		b.vals[index] |= 0x01 << pos
	}

	return true
}

// getBit 获取offset位置处的value值
func (b *Bitmap) getBit(offset uint16) uint8 {
	index, pos := offset/8, offset%8

	if b.size < offset {
		return 0
	}

	return (b.vals[index] >> pos) & 0x01
}
