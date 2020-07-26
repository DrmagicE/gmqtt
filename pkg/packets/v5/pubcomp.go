package v5

import (
	"bytes"
	"fmt"
	"io"
)

// Pubcomp represents the MQTT Pubcomp  packet
type Pubcomp struct {
	FixHeader *FixHeader
	PacketID
	// V5
	Code       byte
	Properties *Properties
}

func (p *Pubcomp) String() string {
	return fmt.Sprintf("Pubcomp, Pid: %v", p.PacketID)
}

// NewPubcompPacket returns a Pubcomp instance by the given FixHeader and io.Reader
func NewPubcompPacket(fh *FixHeader, r io.Reader) (*Pubcomp, error) {
	p := &Pubcomp{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Pubcomp) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBCOMP, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	if p.Code != CodeSuccess || p.Properties != nil {
		bufw.WriteByte(p.Code)
		p.Properties.Pack(bufw, PUBCOMP)
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
func (p *Pubcomp) Unpack(r io.Reader) error {
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
		return err
	}
	if !ValidateCode(PUBCOMP, p.Code) {
		return protocolErr(invalidReasonCode(p.Code))
	}
	return p.Properties.Unpack(bufr, PUBCOMP)
}
