package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Unsubscribe represents the MQTT Unsubscribe  packet.
type Unsubscribe struct {
	FixHeader *FixHeader
	PacketID  PacketID

	Topics []string
}

func (p *Unsubscribe) String() string {
	return fmt.Sprintf("Unsubscribe, Pid: %v, Topics: %v", p.PacketID, p.Topics)
}

// NewUnSubBack returns the Unsuback struct which is the ack packet of the Unsubscribe packet.
func (p *Unsubscribe) NewUnSubBack() *Unsuback {
	fh := &FixHeader{PacketType: UNSUBACK, Flags: 0, RemainLength: 2}
	unSuback := &Unsuback{FixHeader: fh, PacketID: p.PacketID}
	return unSuback
}

// NewUnsubscribePacket returns a Unsubscribe instance by the given FixHeader and io.Reader.
func NewUnsubscribePacket(fh *FixHeader, r io.Reader) (*Unsubscribe, error) {
	p := &Unsubscribe{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-3.10.1-1]
	if fh.Flags != FLAG_UNSUBSCRIBE {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Unsubscribe) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: UNSUBSCRIBE, Flags: FLAG_UNSUBSCRIBE}
	buf := make([]byte, 0, 256)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	buf = append(buf, pid...)
	for _, topic := range p.Topics {
		topicName, _, erro := EncodeUTF8String([]byte(topic))
		buf = append(buf, topicName...)
		if erro != nil {
			return erro
		}
	}
	p.FixHeader.RemainLength = len(buf)
	err := p.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Unsubscribe) Unpack(r io.Reader) error {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return err
	}
	p.PacketID = binary.BigEndian.Uint16(restBuffer[0:2])

	restBuffer = restBuffer[2:]
	for {
		topicName, size, err := DecodeUTF8String(restBuffer)
		if err != nil {
			return err
		}
		if !ValidTopicFilter(topicName) {
			return ErrInvalTopicFilter
		}
		restBuffer = restBuffer[size:]
		p.Topics = append(p.Topics, string(topicName))
		if len(restBuffer) == 0 {
			break
		}
	}

	return nil
}
