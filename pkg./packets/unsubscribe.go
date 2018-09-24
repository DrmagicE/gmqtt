package packets

import (
	"encoding/binary"
	"io"
	"fmt"
)

type Unsubscribe struct {
	FixHeader *FixHeader
	PacketId PacketId

	Topics []string
}


func (c *Unsubscribe) String() string {
	return fmt.Sprintf("Unsubscribe, Pid: %v, Topics: %v",c.PacketId, c.Topics)
}


//suback
func (p *Unsubscribe) NewUnSubBack() *Unsuback{
	fh := &FixHeader{PacketType:UNSUBACK,Flags:0,RemainLength:2}
	unSuback := &Unsuback{FixHeader:fh,PacketId:p.PacketId}
	return unSuback
}

//构建一个subscribe包
func NewUnsubscribePacket(fh *FixHeader,r io.Reader) (*Unsubscribe,error) {
	p := &Unsubscribe{FixHeader: fh}
	//判断 标志位 flags 是否合法[MQTT-3.10.1-1]
	if fh.Flags != FLAG_UNSUBSCRIBE {
		return nil ,ErrInvalFlags
	}
	err := p.Unpack(r)
	return p,err
}

func (c *Unsubscribe) Pack(w io.Writer) error {
	c.FixHeader = &FixHeader{PacketType: UNSUBSCRIBE, Flags: FLAG_UNSUBSCRIBE}
	buf := make([]byte, 0, 256)
	pid := make([]byte, 2)
	binary.BigEndian.PutUint16(pid, c.PacketId)
	buf = append(buf, pid...)
	for _, topic := range c.Topics {
		topicName, _, erro := EncodeUTF8String([]byte(topic))
		buf = append(buf, topicName...)
		if erro != nil {
			return erro
		}
	}
	c.FixHeader.RemainLength = len(buf)
	err := c.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	return err
}

func (p *Unsubscribe) Unpack(r io.Reader) error {
	restBuffer := make([]byte,p.FixHeader.RemainLength)
	_,err := io.ReadFull(r,restBuffer)
	if err!=nil {
		return err
	}
	p.PacketId = binary.BigEndian.Uint16(restBuffer[0:2])

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
		p.Topics = append(p.Topics,string(topicName))
		if len(restBuffer) == 0 {
			break;
		}
	}

	return nil
}
