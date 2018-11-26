package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

//qos 2 step1
type Pubrec struct {
	FixHeader *FixHeader
	PacketId  PacketId
}

func (c *Pubrec) String() string {
	return fmt.Sprintf("Pubrec, Pid: %v", c.PacketId)
}

func NewPubrecPacket(fh *FixHeader, r io.Reader) (*Pubrec, error) {
	p := &Pubrec{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Pubrec) NewPubrel() *Pubrel {
	pub := &Pubrel{FixHeader: &FixHeader{PacketType: PUBREL, Flags: FLAG_PUBREL, RemainLength: 2}}
	pub.PacketId = p.PacketId
	return pub
}

func (p *Pubrec) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBREC, Flags: RESERVED, RemainLength: 2}
	p.FixHeader.Pack(w)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketId)
	_, err := w.Write(pid)
	return err
}

func (p *Pubrec) Unpack(r io.Reader) error {
	if p.FixHeader.RemainLength != 2 {
		return ErrInvalRemainLength
	}
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	p.PacketId = binary.BigEndian.Uint16(restBuffer[0:2])
	return nil
}
