package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Pubrec represents the MQTT Pubrec  packet.
type Pubrec struct {
	Version   Version
	FixHeader *FixHeader
	PacketID  PacketID
	// V5
	Code       byte
	Properties *Properties
}

func (p *Pubrec) String() string {
	return fmt.Sprintf("Pubrec, Version: %v, Code %v, Pid: %v, Properties: %s", p.Version, p.Code, p.PacketID, p.Properties)
}

// NewPubrecPacket returns a Pubrec instance by the given FixHeader and io.Reader.
func NewPubrecPacket(fh *FixHeader, version Version, r io.Reader) (*Pubrec, error) {
	p := &Pubrec{FixHeader: fh, Version: version}
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

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Pubrec) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBREC, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	if p.Version == Version5 && (p.Code != codes.Success || p.Properties != nil) {
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

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Pubrec) Unpack(r io.Reader) error {
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
	if p.Version == Version5 {
		p.Properties = &Properties{}
		if p.Code, err = bufr.ReadByte(); err != nil {
			return codes.ErrMalformed
		}
		if !ValidateCode(PUBREC, p.Code) {
			return codes.ErrProtocol
		}
		return p.Properties.Unpack(bufr, PUBREC)
	}
	return nil

}
