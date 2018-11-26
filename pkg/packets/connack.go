package packets

import (
	"fmt"
	"io"
)

const (
	CODE_ACCEPTED                      = 0x00
	CODE_UNACCEPTABLE_PROTOCOL_VERSION = 0x01
	CODE_IDENTIFIER_REJECTED           = 0x02
	CODE_SERVER_UNAVAILABLE            = 0x03
	CODE_BAD_USERNAME_OR_PSW           = 0x04
	CODE_NOT_AUTHORIZED                = 0x05
)

type Connack struct {
	FixHeader      *FixHeader
	Code           byte
	SessionPresent int
}

func (c *Connack) String() string {
	return fmt.Sprintf("Connack, Code:%v, SessionPresent:%v", c.Code, c.SessionPresent)
}

func (c *Connack) Pack(w io.Writer) error {
	var err error
	c.FixHeader = &FixHeader{PacketType: CONNACK, Flags: FLAG_RESERVED, RemainLength: 2}
	err = c.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	b := make([]byte, 2)
	b[0] = byte(c.SessionPresent)
	b[1] = c.Code
	_, err = w.Write(b)
	return err
}

func (c *Connack) Unpack(r io.Reader) error {
	restBuffer := make([]byte, c.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	if (127 & (restBuffer[0] >> 1)) > 0 {
		return ErrInvalConnAcknowledgeFlags
	}
	if restBuffer[1] > 0 && restBuffer[0] != CODE_ACCEPTED { //[MQTT-3.2.2-4]
		return ErrInvalSessionPresent
	}
	c.SessionPresent = int(restBuffer[0])
	c.Code = restBuffer[1]
	return nil
}

func NewConnackPacket(fh *FixHeader, r io.Reader) (*Connack, error) {
	p := &Connack{FixHeader: fh}
	if fh.Flags != FLAG_RESERVED {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}
