package v5

import (
	"bytes"
	"fmt"
	"io"
)

// Unsuback represents the MQTT Unsuback  packet.
type Unsuback struct {
	FixHeader  *FixHeader
	PacketID   PacketID
	Properties *Properties
	Payload    []ReasonCode
}

func (p *Unsuback) String() string {
	return fmt.Sprintf("Unsuback, Pid: %v", p.PacketID)
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Unsuback) Pack(w io.Writer) error {
	if p.FixHeader == nil {
		p.FixHeader = &FixHeader{PacketType: UNSUBACK, Flags: FlagReserved}
	}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	p.Properties.Pack(bufw, UNSUBACK)
	bufw.Write(p.Payload)
	p.FixHeader.RemainLength = bufw.Len()
	err := p.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Unsuback) Unpack(r io.Reader) error {
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
	p.Properties = &Properties{}
	err = p.Properties.Unpack(bufr, UNSUBACK)
	if err != nil {
		return err
	}
	for {
		b, err := bufr.ReadByte()
		if err != nil {
			return errMalformed(err)
		}
		if !ValidateCode(UNSUBACK, b) {
			return protocolErr(invalidReasonCode(b))
		}
		p.Payload = append(p.Payload, b)
		if bufr.Len() == 0 {
			return nil
		}
	}
}

// NewUnsubackPacket returns a Unsuback instance by the given FixHeader and io.Reader.
func NewUnsubackPacket(fh *FixHeader, r io.Reader) (*Unsuback, error) {
	p := &Unsuback{FixHeader: fh}
	if fh.Flags != FlagReserved {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}
