package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Pubrel represents the MQTT Pubrel  packet
type Pubrel struct {
	FixHeader *FixHeader
	PacketID  PacketID
	// V5
	Code       codes.Code
	Properties *Properties
}

func (p *Pubrel) String() string {
	return fmt.Sprintf("Pubrel, Code: %v, Pid: %v, Properties: %s", p.Code, p.PacketID, p.Properties)
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
	pub := &Pubcomp{FixHeader: &FixHeader{PacketType: PUBCOMP, Flags: FlagReserved, RemainLength: 2}}
	pub.PacketID = p.PacketID
	return pub
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Pubrel) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBREL, Flags: FlagPubrel}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	if p.Code != codes.Success || p.Properties != nil {
		bufw.WriteByte(p.Code)
		p.Properties.Pack(bufw, PUBREL)
	}
	p.FixHeader.RemainLength = bufw.Len()
	err := p.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Pubrel) Unpack(r io.Reader) error {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return codes.ErrMalformed
	}
	bufr := bytes.NewBuffer(restBuffer)
	p.PacketID, err = readUint16(bufr)
	if err != nil {
		return err
	}
	if p.FixHeader.RemainLength == 2 {
		p.Code = codes.Success
		return nil
	}
	p.Properties = &Properties{}
	if p.Code, err = bufr.ReadByte(); err != nil {
		return err
	}
	if !ValidateCode(PUBREL, p.Code) {
		return codes.ErrProtocol
	}
	return p.Properties.Unpack(bufr, PUBREL)
}
