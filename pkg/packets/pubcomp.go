package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Pubcomp struct {
	FixHeader *FixHeader
	PacketID
}

func (c *Pubcomp) String() string {
	return fmt.Sprintf("Pubcomp, Pid: %v", c.PacketID)
}

func NewPubcompPacket(fh *FixHeader, r io.Reader) (*Pubcomp, error) {
	p := &Pubcomp{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Pubcomp) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBCOMP, Flags: RESERVED, RemainLength: 2}
	p.FixHeader.Pack(w)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	_, err := w.Write(pid)
	return err
}

func (p *Pubcomp) Unpack(r io.Reader) error {
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
