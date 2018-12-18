package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Suback represents the MQTT Suback  packet.
type Suback struct {
	FixHeader *FixHeader
	PacketID  PacketID
	Payload   []byte
}

func (p *Suback) String() string {
	return fmt.Sprintf("Suback, Pid: %v, Payload: %v", p.PacketID, p.Payload)
}

// NewSubackPacket returns a Suback instance by the given FixHeader and io.Reader.
func NewSubackPacket(fh *FixHeader, r io.Reader) (*Suback, error) {
	p := &Suback{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-3.8.1-1]
	if fh.Flags != FLAG_RESERVED {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Suback) Pack(w io.Writer) error {
	p.FixHeader.Pack(w)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	w.Write(pid)
	_, err := w.Write(p.Payload)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Suback) Unpack(r io.Reader) error {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	p.PacketID = binary.BigEndian.Uint16(restBuffer[0:2])
	p.Payload = restBuffer[2:]
	return nil
}
