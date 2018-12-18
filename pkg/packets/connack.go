package packets

import (
	"fmt"
	"io"
)

// There are the possible code in the connack packet.
const (
	CodeAccepted                    = 0x00
	CodeUnacceptableProtocolVersion = 0x01
	CodeIdentifierRejected          = 0x02
	CodeServerUnavaliable           = 0x03
	CodeBadUsernameorPsw            = 0x04
	CodeNotAuthorized               = 0x05
)

// Connack represents the MQTT Connack  packet
type Connack struct {
	FixHeader      *FixHeader
	Code           byte
	SessionPresent int
}

func (c *Connack) String() string {
	return fmt.Sprintf("Connack, Code:%v, SessionPresent:%v", c.Code, c.SessionPresent)
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
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

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct
func (c *Connack) Unpack(r io.Reader) error {
	restBuffer := make([]byte, c.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	if (127 & (restBuffer[0] >> 1)) > 0 {
		return ErrInvalConnAcknowledgeFlags
	}
	if restBuffer[1] > 0 && restBuffer[0] != CodeAccepted { //[MQTT-3.2.2-4]
		return ErrInvalSessionPresent
	}
	c.SessionPresent = int(restBuffer[0])
	c.Code = restBuffer[1]
	return nil
}

// NewConnackPacket returns a Connack instance by the given FixHeader and io.Reader
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
