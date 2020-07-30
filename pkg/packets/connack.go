package v5

import (
	"bytes"
	"fmt"
	"io"
)

// Connack represents the MQTT Connack  packet
type Connack struct {
	FixHeader      *FixHeader
	ReasonCode     ReasonCode
	SessionPresent bool
	Properties     *Properties
}

// TODO reasonCode 和returnCode
func (c *Connack) String() string {
	return fmt.Sprintf("Connack, Code:%v, SessionPresent:%v", c.ReasonCode, c.SessionPresent)
}

// Pack encodes the packet struct into bytes and writes it into io.writer.
func (c *Connack) Pack(w io.Writer) error {
	var err error
	c.FixHeader = &FixHeader{PacketType: CONNACK, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	if c.SessionPresent {
		bufw.WriteByte(1)
	} else {
		bufw.WriteByte(0)
	}
	bufw.WriteByte(c.ReasonCode)
	c.Properties.Pack(bufw, CONNACK)
	c.FixHeader.RemainLength = bufw.Len()
	err = c.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.reader and decodes it into the packet struct
func (c *Connack) Unpack(r io.Reader) error {
	restBuffer := make([]byte, c.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return errMalformed(err)
	}
	bufr := bytes.NewBuffer(restBuffer)
	sp, err := bufr.ReadByte()
	if (127 & (sp >> 1)) > 0 {
		return errMalformed(ErrInvalConnAcknowledgeFlags)
	}
	c.SessionPresent = sp == 1

	code, err := bufr.ReadByte()
	if err != nil {
		return errMalformed(err)
	}
	if !ValidateCode(CONNACK, code) {
		return protocolErr(invalidReasonCode(code))
	}
	c.ReasonCode = code
	c.Properties = &Properties{}
	return c.Properties.Unpack(bufr, CONNACK)
}

// NewConnackPacket returns a Connack instance by the given FixHeader and io.reader
func NewConnackPacket(fh *FixHeader, r io.Reader) (*Connack, error) {
	p := &Connack{FixHeader: fh}
	if fh.Flags != FlagReserved {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}
