package packets

import (
	"io"
	"encoding/binary"
	"fmt"
)

type Puback struct {
	FixHeader *FixHeader
	PacketId
}

func (c *Puback) String() string {
	return fmt.Sprintf("Puback, Pid: %v",c.PacketId)
}



//from reader
func NewPubackPacket(fh *FixHeader,r io.Reader) (*Puback,error) {
	p := &Puback{FixHeader:fh}
	err := p.Unpack(r)
	if err != nil {
		return nil,err
	}
	return p,nil
}

func (p *Puback) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType:PUBACK,Flags:RESERVED,RemainLength:2}
	p.FixHeader.Pack(w)
	pid := make([]byte,2)
	binary.BigEndian.PutUint16(pid,p.PacketId)
	_,err := w.Write(pid)
	return err
}

func (p *Puback) Unpack(r io.Reader) error {
	if p.FixHeader.RemainLength != 2 {
		return ErrInvalRemainLength
	}
	restBuffer := make([]byte,p.FixHeader.RemainLength)
	_,err := io.ReadFull(r,restBuffer)
	if err != nil {
		return err
	}
	p.PacketId = binary.BigEndian.Uint16(restBuffer[0:2])
	return nil
}