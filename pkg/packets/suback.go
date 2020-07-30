package v5

import (
	"bytes"
	"fmt"
	"io"
)

// Suback represents the MQTT Suback  packet.
type Suback struct {
	FixHeader  *FixHeader
	PacketID   PacketID
	Payload    []ReasonCode
	Properties *Properties
}

func (p *Suback) String() string {
	return fmt.Sprintf("Suback, Pid: %v, Payload: %v", p.PacketID, p.Payload)
}

// NewSubackPacket returns a Suback instance by the given FixHeader and io.reader.
func NewSubackPacket(fh *FixHeader, r io.Reader) (*Suback, error) {
	p := &Suback{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-3.8.1-1]
	if fh.Flags != FlagReserved {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.writer.
func (p *Suback) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: SUBACK, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	p.Properties.Pack(bufw, SUBACK)
	bufw.Write(p.Payload)
	p.FixHeader.RemainLength = bufw.Len()
	err := p.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err

}

// Unpack read the packet bytes from io.reader and decodes it into the packet struct.
func (p *Suback) Unpack(r io.Reader) error {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return errMalformed(err)
	}
	bufr := bytes.NewBuffer(restBuffer)

	p.PacketID, err = readUint16(bufr)
	if err != nil {
		return errMalformed(err)
	}
	p.Properties = &Properties{}
	err = p.Properties.Unpack(bufr, SUBACK)
	if err != nil {
		return err
	}
	for {
		b, err := bufr.ReadByte()
		if err != nil {
			return errMalformed(err)
		}
		if !ValidateCode(SUBACK, b) {
			return protocolErr(invalidReasonCode(b))
		}
		p.Payload = append(p.Payload, b)
		if bufr.Len() == 0 {
			return nil
		}
	}
}
