package packets

import (
	"bytes"
	"fmt"
	"io"
)

// Pubrel represents the MQTT Pubrel  packet
type Pubrel struct {
	Version   Version
	FixHeader *FixHeader
	PacketID  PacketID

	// V5
	Code       byte
	Properties *Properties
}

func (p *Pubrel) String() string {
	return fmt.Sprintf("Pubrel, Pid: %v", p.PacketID)
}

// NewPubrelPacket returns a Pubrel instance by the given FixHeader and io.Reader.
func NewPubrelPacket(fh *FixHeader, version Version, r io.Reader) (*Pubrel, error) {
	p := &Pubrel{FixHeader: fh, Version: version}
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
	p.FixHeader = &FixHeader{PacketType: PUBREL, Flags: RESERVED}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	bufw.WriteByte(p.Code)
	if p.Properties != nil {
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
		return errMalformed(err)
	}
	bufr := bytes.NewBuffer(restBuffer)
	p.PacketID, err = readUint16(bufr)
	if err != nil {
		return err
	}
	if p.Version == Version5 {
		p.Properties = &Properties{}
		if p.Code, err = bufr.ReadByte(); err != nil {
			return err
		}
		if !ValidateCode(PUBREL, p.Code) {
			return protocolErr(invalidReasonCode(p.Code))
		}
		if err := p.Properties.Unpack(bufr, PUBREL); err != nil {
			return err
		}
	}
	return nil
}
