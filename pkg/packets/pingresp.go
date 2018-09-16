package packets

import (
	"io"
)

type Pingresp struct {
	FixHeader *FixHeader
}

func (p *Pingresp) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PINGRESP, Flags: 0, RemainLength: 0}
	return p.FixHeader.Pack(w)
}

func (p *Pingresp) Unpack(r io.Reader) error {
	if p.FixHeader.RemainLength != 0 {
		return ErrInvalRemainLength
	}
	return nil
}

func NewPingrespPacket(fh *FixHeader, r io.Reader) (*Pingresp, error) {
	if fh.Flags != FLAG_RESERVED {
		return nil, ErrInvalFlags
	}
	p := &Pingresp{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}