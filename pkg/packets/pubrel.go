package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Pubrel represents the MQTT Pubrel  packet
type Pubrel struct {
	FixHeader *FixHeader
	PacketID  PacketID

	//Dup bool //是否重发标记，不属于协议包内容
}

func (p *Pubrel) String() string {
	return fmt.Sprintf("Pubrel, Pid: %v", p.PacketID)
}

// NewPubrelPacket returns a Pubrel instance by the given FixHeader and io.Reader.
func NewPubrelPacket(fh *FixHeader, r io.Reader) (*Pubrel, error) {
	p := &Pubrel{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// NewPubcomp returns the Pubcomp struct related to the Pubrel struct in QoS 2.
func (p *Pubrel) NewPubcomp() *Pubcomp {
	pub := &Pubcomp{FixHeader: &FixHeader{PacketType: PUBCOMP, Flags: RESERVED, RemainLength: 2}}
	pub.PacketID = p.PacketID
	return pub
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Pubrel) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBREL, Flags: FLAG_PUBREL, RemainLength: 2}
	p.FixHeader.Pack(w)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	_, err := w.Write(pid)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Pubrel) Unpack(r io.Reader) error {
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
