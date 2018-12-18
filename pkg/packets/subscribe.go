package packets

import (
	"encoding/binary"
	"fmt"
	"io"
)

type Subscribe struct {
	FixHeader *FixHeader
	PacketID  PacketID

	Topics []Topic //suback响应之前填充
}

func (c *Subscribe) String() string {
	str := fmt.Sprintf("Subscribe, Pid: %v", c.PacketID)

	for k, t := range c.Topics {
		str += fmt.Sprintf(", Topic[%d][Name: %s, Qos: %v]", k, t.Name, t.Qos)
	}
	return str
}

//suback
func (p *Subscribe) NewSubBack() *Suback {
	fh := &FixHeader{PacketType: SUBACK, Flags: FLAG_RESERVED}
	suback := &Suback{FixHeader: fh, Payload: make([]byte, 0, len(p.Topics))}
	suback.PacketID = p.PacketID
	var qos byte
	for _, v := range p.Topics {
		qos = v.Qos
		suback.Payload = append(suback.Payload, qos)
	}
	fh.RemainLength = 2 + len(suback.Payload)
	return suback
}

//new subscribe
func NewSubscribePacket(fh *FixHeader, r io.Reader) (*Subscribe, error) {
	p := &Subscribe{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-3.8.1-1]
	if fh.Flags != FLAG_SUBSCRIBE {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	return p, err
}

func (p *Subscribe) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: SUBSCRIBE, Flags: FLAG_SUBSCRIBE}
	buf := make([]byte, 0, 256)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, p.PacketID)
	buf = append(buf, pid...)
	for _, t := range p.Topics {
		topicName, _, _ := EncodeUTF8String([]byte(t.Name))
		buf = append(buf, topicName...)
		buf = append(buf, t.Qos)
	}
	p.FixHeader.RemainLength = len(buf)
	p.FixHeader.Pack(w)
	_, err := w.Write(buf)
	return err

}

func (p *Subscribe) Unpack(r io.Reader) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrInvalUTF8String
		}
	}()
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err = io.ReadFull(r, restBuffer)
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
		qos := uint8(restBuffer[0])
		restBuffer = restBuffer[1:]
		if qos > QOS_2 {
			return ErrInvalQos
		}
		p.Topics = append(p.Topics, Topic{Name: string(topicName), Qos: qos})
		if len(restBuffer) == 0 {
			break
		}
	}

	return nil
}
