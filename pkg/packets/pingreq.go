package packets

import (
	"io"
	"fmt"
)

type Pingreq struct {
	FixHeader *FixHeader
}

func (c *Pingreq) String() string {
	return fmt.Sprintf("Pingreq")
}


func NewPingreqPacket(fh *FixHeader,r io.Reader) (*Pingreq,error) {
	if fh.Flags != FLAG_RESERVED {
		return nil,ErrInvalFlags
	}
	p := &Pingreq{FixHeader:fh}
	err := p.Unpack(r)
	if err != nil {
		return nil,err
	}
	return p,nil
}

func (p *Pingreq) NewPingresp () *Pingresp{
	fh := &FixHeader{PacketType:PINGRESP,Flags:0,RemainLength:0}
	return &Pingresp{FixHeader:fh}
}


func (p *Pingreq) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PINGREQ, Flags: 0, RemainLength: 0}
	return p.FixHeader.Pack(w)
}


func (p *Pingreq) Unpack(r io.Reader) error {
	if p.FixHeader.RemainLength != 0 {
		return ErrInvalRemainLength
	}
	return nil
}