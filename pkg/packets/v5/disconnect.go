package v5

import (
	"bytes"
	"fmt"
	"io"
)

// Disconnect represents the MQTT Disconnect  packet
type Disconnect struct {
	FixHeader  *FixHeader
	Code       ReasonCode
	Properties *Properties
}

func (d *Disconnect) String() string {
	return fmt.Sprintf("Disconnect")
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (d *Disconnect) Pack(w io.Writer) error {
	var err error
	d.FixHeader = &FixHeader{PacketType: DISCONNECT, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	if d.Code != CodeSuccess || d.Properties != nil {
		bufw.WriteByte(d.Code)
		d.Properties.Pack(bufw, DISCONNECT)
	}
	d.FixHeader.RemainLength = bufw.Len()
	err = d.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (d *Disconnect) Unpack(r io.Reader) error {
	restBuffer := make([]byte, d.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return errMalformed(err)
	}
	bufr := bytes.NewBuffer(restBuffer)
	if d.FixHeader.RemainLength == 0 {
		d.Code = CodeSuccess
		return nil
	}
	d.Code, err = bufr.ReadByte()
	if err != nil {
		return errMalformed(err)
	}
	if !ValidateCode(DISCONNECT, d.Code) {
		return protocolErr(invalidReasonCode(d.Code))
	}
	d.Properties = &Properties{}
	return d.Properties.Unpack(bufr, DISCONNECT)

}

// NewDisConnectPackets returns a Disconnect instance by the given FixHeader and io.Reader
func NewDisConnectPackets(fh *FixHeader, r io.Reader) (*Disconnect, error) {
	if fh.Flags != 0 {
		return nil, errMalformed(ErrInvalFlags)
	}
	p := &Disconnect{FixHeader: fh}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}
