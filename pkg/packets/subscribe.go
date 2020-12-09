package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Subscribe represents the MQTT Subscribe  packet.
type Subscribe struct {
	Version    Version
	FixHeader  *FixHeader
	PacketID   PacketID
	Topics     []Topic //suback响应之前填充
	Properties *Properties
}

func (p *Subscribe) String() string {
	str := fmt.Sprintf("Subscribe, Version: %v, Pid: %v", p.Version, p.PacketID)
	for k, t := range p.Topics {
		str += fmt.Sprintf(", Topic[%d][Name: %s, Qos: %v]", k, t.Name, t.Qos)
	}
	str += fmt.Sprintf(", Properties: %s", p.Properties)
	return str
}

// NewSuback returns the Suback struct which is the ack packet of the Subscribe packet.
func (p *Subscribe) NewSuback() *Suback {
	fh := &FixHeader{PacketType: SUBACK, Flags: FlagReserved}
	suback := &Suback{FixHeader: fh, Version: p.Version, Payload: make([]byte, 0, len(p.Topics))}
	suback.PacketID = p.PacketID
	var qos byte
	for _, v := range p.Topics {
		qos = v.Qos
		suback.Payload = append(suback.Payload, qos)
	}
	return suback
}

// NewSubscribePacket returns a Subscribe instance by the given FixHeader and io.Reader.
func NewSubscribePacket(fh *FixHeader, version Version, r io.Reader) (*Subscribe, error) {
	p := &Subscribe{FixHeader: fh, Version: version}
	//判断 标志位 flags 是否合法[MQTT-3.8.1-1]
	if fh.Flags != FlagSubscribe {
		return nil, codes.ErrMalformed
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Subscribe) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: SUBSCRIBE, Flags: FlagSubscribe}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	var nl, rap byte
	if p.Version == Version5 {
		p.Properties.Pack(bufw, SUBSCRIBE)
		for _, v := range p.Topics {
			writeUTF8String(bufw, []byte(v.Name))
			if v.NoLocal {
				nl = 4
			} else {
				nl = 0
			}
			if v.RetainAsPublished {
				rap = 8
			} else {
				rap = 0
			}
			bufw.WriteByte(v.Qos | nl | rap | (v.RetainHandling << 4))
		}
	} else {
		for _, t := range p.Topics {
			writeUTF8String(bufw, []byte(t.Name))
			bufw.WriteByte(t.Qos)
		}
	}
	p.FixHeader.RemainLength = bufw.Len()
	err := p.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err

}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Subscribe) Unpack(r io.Reader) (err error) {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err = io.ReadFull(r, restBuffer)
	if err != nil {
		return codes.ErrMalformed
	}
	bufr := bytes.NewBuffer(restBuffer)
	p.PacketID, err = readUint16(bufr)
	if err != nil {
		return err
	}
	if p.Version == Version5 {
		p.Properties = &Properties{}
		if err := p.Properties.Unpack(bufr, SUBSCRIBE); err != nil {
			return err
		}
	}
	for {
		topicFilter, err := readUTF8String(true, bufr)
		if err != nil {
			return err
		}
		if p.Version == Version5 {
			// check shared subscription syntax
			if !ValidV5Topic(topicFilter) {
				return codes.ErrMalformed
			}
		} else {
			if !ValidTopicFilter(true, topicFilter) {
				return codes.ErrMalformed
			}
		}
		opts, err := bufr.ReadByte()
		if err != nil {
			return codes.ErrMalformed
		}
		topic := Topic{
			Name: string(topicFilter),
		}
		if p.Version == Version5 {
			topic.Qos = opts & 3
			topic.NoLocal = (1 & (opts >> 2)) > 0
			topic.RetainAsPublished = (1 & (opts >> 3)) > 0
			topic.RetainHandling = (3 & (opts >> 4))
		} else {
			topic.Qos = opts
			if topic.Qos > Qos2 {
				return codes.ErrProtocol
			}
		}

		// check reserved
		if 3&(opts>>6) != 0 {
			return codes.ErrProtocol
		}
		if topic.Qos > Qos2 {
			return codes.ErrProtocol
		}
		p.Topics = append(p.Topics, topic)
		if bufr.Len() == 0 {
			return nil
		}
	}
}
