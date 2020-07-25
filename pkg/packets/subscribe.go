package packets

import (
	"bytes"
	"fmt"
	"io"
)




// Subscribe represents the MQTT Subscribe  packet.
type Subscribe struct {
	Version   Version
	FixHeader *FixHeader
	PacketID  PacketID

	Topics []Topic //suback响应之前填充
	//V5
	Properties *Properties
}

func (p *Subscribe) String() string {
	str := fmt.Sprintf("Subscribe, Pid: %v", p.PacketID)

	for k, t := range p.Topics {
		str += fmt.Sprintf(", Topic[%d][Name: %s, Qos: %v]", k, t.Name, t.Qos)
	}
	return str
}



// NewSuback returns the Suback struct which is the ack packet of the Subscribe packet.
func (p *Subscribe) NewSuback() *Suback {
	// TODO, 改成根据granted Qos来生成,
	fh := &FixHeader{PacketType: SUBACK, Flags: FLAG_RESERVED}
	suback := &Suback{FixHeader: fh, Payload: make([]byte, 0, len(p.Topics)), Version: p.Version}
	suback.PacketID = p.PacketID
	var qos byte
	for _, v := range p.Topics {
		qos = v.Qos
		suback.Payload = append(suback.Payload, qos)
	}
	// todo
	return suback
}

// NewSubscribePacket returns a Subscribe instance by the given FixHeader and io.Reader.
func NewSubscribePacket(fh *FixHeader, version Version, r io.Reader) (*Subscribe, error) {
	p := &Subscribe{FixHeader: fh, Version: version}
	//判断 标志位 flags 是否合法[MQTT-3.8.1-1]
	if fh.Flags != FLAG_SUBSCRIBE {
		return nil, ErrInvalFlags
	}
	err := p.Unpack(r)
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Subscribe) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: SUBSCRIBE, Flags: FLAG_SUBSCRIBE}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	if p.Version == Version_5 {
		var nl, rap byte
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
	p.FixHeader.Pack(w)
	_, err := bufw.WriteTo(w)
	return err

}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (p *Subscribe) Unpack(r io.Reader) (err error) {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err = io.ReadFull(r, restBuffer)
	if err != nil {
		return errMalformed(err)
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
		for {
			topicFilter, err := readUTF8String(true, bufr)
			if err != nil {
				return err
			}
			if !ValidTopicFilter(true, topicFilter) {
				return errCode{code:ReasonCodeTopicFilterInvalid}
			}
			opts, err := bufr.ReadByte()
			if err != nil {
				return errMalformed(err)
			}
			topic := Topic{
				Name: string(topicFilter),
			}

			topic.Qos = opts & 3
			topic.NoLocal = (1 & (opts >> 2)) > 0
			topic.RetainAsPublished = (1 & (opts >> 3)) > 0
			topic.RetainHandling = (3 & (opts >> 4))
			// check reserved
			if 3&(opts>>6) != 0 {
				return protocolErr(nil)
			}
			if topic.Qos > QOS_2 {
				return protocolErr(ErrInvalQos)
			}
			p.Topics = append(p.Topics, topic)
			if bufr.Len() == 0 {
				return nil
			}
		}

	}

	// v311
	for {
		topicFilter, err := readUTF8String(true, bufr)
		if err != nil {
			return err
		}
		if !ValidTopicFilter(true, topicFilter) {
			return errCode{code:ReasonCodeTopicFilterInvalid}
		}
		qos, err := bufr.ReadByte()
		if err != nil {
			return errMalformed(err)
		}
		if qos > QOS_2 {
			//return ErrInvalQos
			return protocolErr(ErrInvalQos)
		}
		p.Topics = append(p.Topics, Topic{Name: string(topicFilter), SubOptions:SubOptions{Qos:qos}})
		if bufr.Len() == 0 {
			return nil
		}
	}
}
