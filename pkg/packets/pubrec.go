package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Pubrec represents the MQTT Pubrec  packet.
type Pubrec struct {
	FixHeader *FixHeader
	PacketID  PacketID
}

func (p *Pubrec) String() string {
	return fmt.Sprintf("Pubrec, Pid: %v", p.PacketID)
}

// NewPubrecPacket returns a Pubrec instance by the given FixHeader and io.Reader.
func NewPubrecPacket(fh *FixHeader, r io.Reader) (*Pubrec, error) {
	p := &Pubrec{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// NewPubrel returns the Pubrel struct related to the Pubrec struct in QoS 2.
func (p *Pubrec) NewPubrel() *Pubrel {
	pub := &Pubrel{FixHeader: &FixHeader{PacketType: PUBREL, Flags: FLAG_PUBREL, RemainLength: 2}}
	pub.PacketID = p.PacketID
	return pub
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Pubrec) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBREC, Flags: RESERVED, RemainLength: 2}
	p.FixHeader.Pack(w)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	_, err := w.Write(pid)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Pubrec) Unpack(r io.Reader) error {
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
