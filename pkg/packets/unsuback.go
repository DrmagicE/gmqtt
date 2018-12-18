package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

//响应
type Unsuback struct {
	FixHeader *FixHeader
	PacketID  PacketID
}

func (c *Unsuback) String() string {
	return fmt.Sprintf("Unsuback, Pid: %v", c.PacketID)
}

func (p *Unsuback) Pack(w io.Writer) error {
	if p.FixHeader == nil {
		p.FixHeader = &FixHeader{PacketType: UNSUBACK, Flags: FLAG_RESERVED, RemainLength: 2}
	}
	err := p.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	_, err = w.Write(pid)
	return err
}

func (p *Unsuback) Unpack(r io.Reader) error {
	if p.FixHeader.RemainLength != 2 {
		return ErrInvalRemainLength
	}
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	p.PacketID = binary.BigEndian.Uint16(restBuffer[0:2])
	return nil
}

func NewUnsubackPacket(fh *FixHeader, r io.Reader) (*Unsuback, error) {
	p := &Unsuback{FixHeader: fh}
	if fh.Flags != FLAG_RESERVED {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}
