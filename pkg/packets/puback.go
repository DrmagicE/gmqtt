package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Puback represents the MQTT Puback  packet
type Puback struct {
	FixHeader *FixHeader
	PacketID
}

func (p *Puback) String() string {
	return fmt.Sprintf("Puback, Pid: %v", p.PacketID)
}

// NewPubackPacket returns a Puback instance by the given FixHeader and io.Reader
func NewPubackPacket(fh *FixHeader, r io.Reader) (*Puback, error) {
	p := &Puback{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Puback) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBACK, Flags: RESERVED, RemainLength: 2}
	p.FixHeader.Pack(w)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	_, err := w.Write(pid)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Puback) Unpack(r io.Reader) error {
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
