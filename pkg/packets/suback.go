package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Suback represents the MQTT Suback  packet.
type Suback struct {
	Version    Version
	FixHeader  *FixHeader
	PacketID   PacketID
	Payload    []codes.Code
	Properties *Properties
}

func (p *Suback) String() string {
	return fmt.Sprintf("Suback,Version: %v, Pid: %v, Payload: %v, Properties: %s", p.Version, p.PacketID, p.Payload, p.Properties)
}

// NewSubackPacket returns a Suback instance by the given FixHeader and io.Reader.
func NewSubackPacket(fh *FixHeader, version Version, r io.Reader) (*Suback, error) {
	p := &Suback{FixHeader: fh, Version: version}
	//判断 标志位 flags 是否合法[MQTT-3.8.1-1]
	if fh.Flags != FlagReserved {
		return nil, codes.ErrMalformed
	}
	err := p.Unpack(r)
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Suback) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: SUBACK, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	if p.Version == Version5 {
		p.Properties.Pack(bufw, SUBACK)
	}
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
func (p *Suback) Unpack(r io.Reader) error {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return codes.ErrMalformed
	}
	bufr := bytes.NewBuffer(restBuffer)

	p.PacketID, err = readUint16(bufr)
	if err != nil {
		return codes.ErrMalformed
	}
	if p.Version == Version5 {
		p.Properties = &Properties{}
		err = p.Properties.Unpack(bufr, SUBACK)
		if err != nil {
			return err
		}
	}
	for {
		b, err := bufr.ReadByte()
		if err != nil {
			return codes.ErrMalformed
		}
		if !ValidateCode(SUBACK, b) {
			return codes.ErrProtocol
		}
		p.Payload = append(p.Payload, b)
		if bufr.Len() == 0 {
			return nil
		}
	}
}
