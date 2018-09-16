package packets

import (
	"io"
	"encoding/binary"
)

type Suback struct {
	FixHeader *FixHeader
	PacketId PacketId
	Payload []byte
}

//new suback
func NewSubackPacket(fh *FixHeader,r io.Reader) (*Suback,error) {
	p := &Suback{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-3.8.1-1]
	if fh.Flags != FLAG_RESERVED {
		return nil ,ErrInvalFlags
	}
	err := p.Unpack(r)
	return p,err
}

func (p *Suback) Pack(w io.Writer) error {
	p.FixHeader.Pack(w)
	pid := make([]byte,2)
	binary.BigEndian.PutUint16(pid,p.PacketId)
	w.Write(pid)
	_,err := w.Write(p.Payload)
	return err
}

func (p *Suback) Unpack(r io.Reader) error {
	restBuffer := make([]byte,p.FixHeader.RemainLength)
	_,err := io.ReadFull(r,restBuffer)
	if err!=nil {
		return err
	}
	p.PacketId = binary.BigEndian.Uint16(restBuffer[0:2])
	p.Payload = restBuffer[2:]
	return nil
}

