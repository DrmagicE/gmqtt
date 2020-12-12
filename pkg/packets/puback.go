package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Puback represents the MQTT Puback  packet
type Puback struct {
	Version   Version
	FixHeader *FixHeader
	PacketID  PacketID
	// V5
	Code       codes.Code
	Properties *Properties
}

func (p *Puback) String() string {
	return fmt.Sprintf("Puback, Version: %v, Pid: %v, Properties: %s", p.Version, p.PacketID, p.Properties)
}

// NewPubackPacket returns a Puback instance by the given FixHeader and io.Reader
func NewPubackPacket(fh *FixHeader, version Version, r io.Reader) (*Puback, error) {
	p := &Puback{FixHeader: fh, Version: version}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Puback) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: PUBACK, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	if p.Version == Version5 && (p.Code != codes.Success || p.Properties != nil) {
		bufw.WriteByte(p.Code)
		p.Properties.Pack(bufw, PUBACK)

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
func (p *Puback) Unpack(r io.Reader) error {
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
		if !ValidateCode(PUBACK, p.Code) {
			return codes.ErrProtocol
		}
		if err := p.Properties.Unpack(bufr, PUBACK); err != nil {
			return err
		}
	}
	return nil
}
