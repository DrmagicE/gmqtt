package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Disconnect represents the MQTT Disconnect  packet
type Disconnect struct {
	Version   Version
	FixHeader *FixHeader
	// V5
	Code       codes.Code
	Properties *Properties
}

func (d *Disconnect) String() string {
	return fmt.Sprintf("Disconnect, Version: %v, Code: %v, Properties: %s", d.Version, d.Code, d.Properties)
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (d *Disconnect) Pack(w io.Writer) error {
	var err error
	d.FixHeader = &FixHeader{PacketType: DISCONNECT, Flags: FlagReserved}
	if IsVersion3X(d.Version) {
		d.FixHeader.RemainLength = 0
		return d.FixHeader.Pack(w)
	}
	bufw := &bytes.Buffer{}
	if d.Code != codes.Success || d.Properties != nil {
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
		return codes.ErrMalformed
	}
	if d.Version == Version5 {
		d.Properties = &Properties{}
		bufr := bytes.NewBuffer(restBuffer)
		if d.FixHeader.RemainLength == 0 {
			d.Code = codes.Success
			return nil
		}
		d.Code, err = bufr.ReadByte()
		if err != nil {
			return codes.ErrMalformed
		}
		if !ValidateCode(DISCONNECT, d.Code) {
			return codes.ErrProtocol
		}
		return d.Properties.Unpack(bufr, DISCONNECT)
	}
	return nil
}

// NewDisConnectPackets returns a Disconnect instance by the given FixHeader and io.Reader
func NewDisConnectPackets(fh *FixHeader, version Version, r io.Reader) (*Disconnect, error) {
	if fh.Flags != 0 {
		return nil, codes.ErrMalformed
	}
	p := &Disconnect{FixHeader: fh, Version: version}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, nil
}
