package v5

import (
	`bytes`
	`fmt`
	`io`
)

type Auth struct {
	FixHeader *FixHeader
	Code byte
	Properties *Properties
}

func (a *Auth) String() string {
	return fmt.Sprintf("Auth")
}


func (a *Auth) Pack(w io.Writer) error {
	a.FixHeader = &FixHeader{PacketType: AUTH, Flags: FlagReserved}
	bufw := &bytes.Buffer{}
	bufw.WriteByte(a.Code)
	a.Properties.Pack(bufw, AUTH)
	a.FixHeader.RemainLength = bufw.Len()
	err := a.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

func (a *Auth) Unpack(r io.Reader) error {
	restBuffer := make([]byte, a.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	bufr := bytes.NewBuffer(restBuffer)
	a.Code, err = bufr.ReadByte()
	if err != nil {
		return err
	}
	if !ValidateCode(AUTH, a.Code) {
		return protocolErr(invalidReasonCode(a.Code))
	}
	a.Properties = &Properties{}
	return a.Properties.Unpack(bufr, AUTH)
}