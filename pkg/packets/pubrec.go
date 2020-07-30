package v5

import (
	"bytes"
	"fmt"
	"io"
)

// Pubrec represents the MQTT Pubrec  packet.
type Pubrec struct {
	FixHeader *FixHeader
	PacketID  PacketID
	// V5
	Code       byte
	Properties *Properties
}

func (p *Pubrec) String() string {
	return fmt.Sprintf("Pubrec, Pid: %v", p.PacketID)
}

// NewPubrecPacket returns a Pubrec instance by the given FixHeader and io.reader.
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
	pub := &Pubrel{FixHeader: &FixHeader{PacketType: PUBREL, Flags: FlagPubrel}}
	pub.PacketID = p.PacketID
	return pub
}

// Pack encodes the packet struct into bytes and writes it into io.writer.
func (p *Pubrec) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBREC, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	if p.Code != CodeSuccess || p.Properties != nil {
		bufw.WriteByte(p.Code)
		p.Properties.Pack(bufw, PUBREC)
	}
	p.FixHeader.RemainLength = bufw.Len()
	err := p.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.reader and decodes it into the packet struct.
func (p *Pubrec) Unpack(r io.Reader) error {
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
	if p.FixHeader.RemainLength == 2 {
		p.Code = CodeSuccess
		return nil
	}
	p.Properties = &Properties{}
	if p.Code, err = bufr.ReadByte(); err != nil {
		return errMalformed(err)
	}
	if !ValidateCode(PUBREC, p.Code) {
		return protocolErr(invalidReasonCode(p.Code))
	}
	return p.Properties.Unpack(bufr, PUBREC)
}
