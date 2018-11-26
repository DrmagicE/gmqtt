package packets

import (
	"fmt"
	"io"
)

type Disconnect struct {
	FixHeader *FixHeader
}

func (c *Disconnect) String() string {
	return fmt.Sprintf("Disconnect")
}

func (d *Disconnect) Pack(w io.Writer) error {
	var err error
	d.FixHeader = &FixHeader{PacketType: DISCONNECT, Flags: FLAG_RESERVED, RemainLength: 0}
	err = d.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	return nil
}
func (d *Disconnect) Unpack(r io.Reader) error {
	if d.FixHeader.RemainLength != 0 {
		return ErrInvalRemainLength
	}
	return nil
}

//构建一个connect包
func NewDisConnectPackets(fh *FixHeader, r io.Reader) (*Disconnect, error) {
	if fh.Flags != 0 {
		return nil, ErrInvalFlags
	}
	p := &Disconnect{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}
