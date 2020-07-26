package v5

import (
	"bytes"
	"fmt"
	"io"
)

// Unsubscribe represents the MQTT Unsubscribe  packet.
type Unsubscribe struct {
	FixHeader  *FixHeader
	PacketID   PacketID
	Topics     []string
	Properties *Properties
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
	if fh.Flags != FlagUnsubscribe {
		return nil, errMalformed(ErrInvalFlags)
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (p *Unsubscribe) Pack(w io.Writer) error {
	p.FixHeader = &FixHeader{PacketType: UNSUBSCRIBE, Flags: FlagUnsubscribe}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, p.PacketID)
	p.Properties.Pack(bufw, UNSUBSCRIBE)
	for _, topic := range p.Topics {
		writeUTF8String(bufw, []byte(topic))
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
func (p *Unsubscribe) Unpack(r io.Reader) error {
	restBuffer := make([]byte, p.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return errMalformed(err)
	}
	bufr := bytes.NewBuffer(restBuffer)
	p.PacketID, err = readUint16(bufr)
	if err != nil {
		return err
	}
	p.Properties = &Properties{}
	if err := p.Properties.Unpack(bufr, UNSUBSCRIBE); err != nil {
		return err
	}
	for {
		topicFilter, err := readUTF8String(true, bufr)
		if err != nil {
			return err
		}
		if !ValidTopicFilter(true, topicFilter) {
			return protocolErr(ErrInvalTopicFilter)
		}
		p.Topics = append(p.Topics, string(topicFilter))
		if bufr.Len() == 0 {
			return nil
		}
	}
}
