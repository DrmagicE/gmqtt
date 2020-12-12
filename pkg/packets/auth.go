package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

type Auth struct {
	FixHeader  *FixHeader
	Code       byte
	Properties *Properties
}

func (a *Auth) String() string {
	return fmt.Sprintf("Auth, Code: %v, Properties: %s", a.Code, a.Properties)
}

func (a *Auth) Pack(w io.Writer) error {
	a.FixHeader = &FixHeader{PacketType: AUTH, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	if a.Code != codes.Success || a.Properties != nil {
		bufw.WriteByte(a.Code)
		a.Properties.Pack(bufw, AUTH)
	}
	a.FixHeader.RemainLength = bufw.Len()
	err := a.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

func (a *Auth) Unpack(r io.Reader) error {
	if a.FixHeader.RemainLength == 0 {
		a.Code = codes.Success
		return nil
	}
	restBuffer := make([]byte, a.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return codes.ErrMalformed
	}
	bufr := bytes.NewBuffer(restBuffer)
	a.Code, err = bufr.ReadByte()
	if err != nil {
		return codes.ErrMalformed
	}
	if !ValidateCode(AUTH, a.Code) {
		return codes.ErrProtocol
	}
	a.Properties = &Properties{}
	return a.Properties.Unpack(bufr, AUTH)
}

func NewAuthPacket(fh *FixHeader, r io.Reader) (*Auth, error) {
	p := &Auth{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-2.2.2-2]
	if fh.Flags != FlagReserved {
		return nil, codes.ErrMalformed
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}
