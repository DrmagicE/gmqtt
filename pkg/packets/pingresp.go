package v5

import (
	"fmt"
	"io"
)

// Pingresp represents the MQTT Pingresp  packet
type Pingresp struct {
	FixHeader *FixHeader
}

func (p *Pingresp) String() string {
	return fmt.Sprintf("Pingresp")
}

// Pack encodes the packet struct into bytes and writes it into io.writer.
func (p *Pingresp) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PINGRESP, Flags: 0, RemainLength: 0}
	return p.FixHeader.Pack(w)
}

// Unpack read the packet bytes from io.reader and decodes it into the packet struct.
func (p *Pingresp) Unpack(r io.Reader) error {
	if p.FixHeader.RemainLength != 0 {
		return errMalformed(ErrInvalRemainLength)
	}
	return nil
}

// NewPingrespPacket returns a Pingresp instance by the given FixHeader and io.reader
func NewPingrespPacket(fh *FixHeader, r io.Reader) (*Pingresp, error) {
	if fh.Flags != FlagReserved {
		return nil, ErrInvalFlags
	}
	p := &Pingresp{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}
