package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Connack represents the MQTT Connack  packet
type Connack struct {
	Version        Version
	FixHeader      *FixHeader
	Code           codes.Code
	SessionPresent bool
	Properties     *Properties
}

func (c *Connack) String() string {
	return fmt.Sprintf("Connack, Version: %v, Code:%v, SessionPresent:%v, Properties: %s", c.Version, c.Code, c.SessionPresent, c.Properties)
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (c *Connack) Pack(w io.Writer) error {
	var err error
	c.FixHeader = &FixHeader{PacketType: CONNACK, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	if c.SessionPresent {
		bufw.WriteByte(1)
	} else {
		bufw.WriteByte(0)
	}
	bufw.WriteByte(c.Code)
	if c.Version == Version5 {
		c.Properties.Pack(bufw, CONNACK)
	}
	c.FixHeader.RemainLength = bufw.Len()
	err = c.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct
func (c *Connack) Unpack(r io.Reader) error {
	restBuffer := make([]byte, c.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return codes.ErrMalformed
	}
	bufr := bytes.NewBuffer(restBuffer)
	sp, err := bufr.ReadByte()
	if (127 & (sp >> 1)) > 0 {
		return codes.ErrMalformed
	}
	c.SessionPresent = sp == 1

	code, err := bufr.ReadByte()
	if err != nil {
		return codes.ErrMalformed
	}

	c.Code = code
	if c.Version == Version5 {
		if !ValidateCode(CONNACK, code) {
			return codes.ErrProtocol
		}
		c.Properties = &Properties{}
		return c.Properties.Unpack(bufr, CONNACK)
	}
	return nil

}

// NewConnackPacket returns a Connack instance by the given FixHeader and io.Reader
func NewConnackPacket(fh *FixHeader, version Version, r io.Reader) (*Connack, error) {
	p := &Connack{FixHeader: fh, Version: Version5}
	if fh.Flags != FlagReserved {
		return nil, codes.ErrMalformed
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}
