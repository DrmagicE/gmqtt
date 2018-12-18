package packets

import (
	"fmt"
	"io"
)

// Disconnect represents the MQTT Disconnect  packet
type Disconnect struct {
	FixHeader *FixHeader
}

func (d *Disconnect) String() string {
	return fmt.Sprintf("Disconnect")
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (d *Disconnect) Pack(w io.Writer) error {
	var err error
	d.FixHeader = &FixHeader{PacketType: DISCONNECT, Flags: FLAG_RESERVED, RemainLength: 0}
	err = d.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	return nil
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (d *Disconnect) Unpack(r io.Reader) error {
	if d.FixHeader.RemainLength != 0 {
		return ErrInvalRemainLength
	}
	return nil
}

// NewDisConnectPackets returns a Disconnect instance by the given FixHeader and io.Reader
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
