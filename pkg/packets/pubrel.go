package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

//qos2 step2
type Pubrel struct {
	FixHeader *FixHeader
	PacketId  PacketId

	Dup bool //是否重发标记，不属于协议包内容
}

func (c *Pubrel) String() string {
	return fmt.Sprintf("Pubrel, Pid: %v", c.PacketId)
}

func NewPubrelPacket(fh *FixHeader, r io.Reader) (*Pubrel, error) {
	p := &Pubrel{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Pubrel) NewPubcomp() *Pubcomp {
	pub := &Pubcomp{FixHeader: &FixHeader{PacketType: PUBCOMP, Flags: RESERVED, RemainLength: 2}}
	pub.PacketId = p.PacketId
	return pub
}

func (p *Pubrel) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBREL, Flags: FLAG_PUBREL, RemainLength: 2}
	p.FixHeader.Pack(w)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketId)
	_, err := w.Write(pid)
	return err
}

func (p *Pubrel) Unpack(r io.Reader) error {
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
