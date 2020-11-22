package queue

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/persistence/encoding"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type MessageWithID interface {
	ID() packets.PacketID
	SetID(id packets.PacketID)
}

type Publish struct {
	*gmqtt.Message
}

func (p *Publish) ID() packets.PacketID {
	return p.PacketID
}
func (p *Publish) SetID(id packets.PacketID) {
	p.PacketID = id
}

type Pubrel struct {
	PacketID packets.PacketID
}

func (p *Pubrel) ID() packets.PacketID {
	return p.PacketID
}
func (p *Pubrel) SetID(id packets.PacketID) {
	p.PacketID = id
}

// Elem represents the element store in the queue.
type Elem struct {
	// At represents the entry time.
	At time.Time
	// Expiry represents the expiry time.
	// Empty means never expire.
	Expiry time.Time
	MessageWithID
}

// Encode encodes the publish structure into bytes and write it to the buffer
func (p *Publish) Encode(b *bytes.Buffer) {
	encoding.WriteBool(b, p.Dup)
	b.WriteByte(p.QoS)
	encoding.WriteBool(b, p.Retained)
	encoding.WriteString(b, []byte(p.Topic))
	encoding.WriteString(b, []byte(p.Payload))
	encoding.WriteUint16(b, p.PacketID)

	if len(p.ContentType) != 0 {
		b.WriteByte(packets.PropContentType)
		encoding.WriteString(b, []byte(p.ContentType))
	}
	if len(p.CorrelationData) != 0 {
		b.WriteByte(packets.PropCorrelationData)
		encoding.WriteString(b, []byte(p.CorrelationData))
	}
	if p.MessageExpiry != 0 {
		b.WriteByte(packets.PropMessageExpiry)
		encoding.WriteUint32(b, p.MessageExpiry)
	}
	b.WriteByte(packets.PropPayloadFormat)
	b.WriteByte(p.PayloadFormat)

	if len(p.ResponseTopic) != 0 {
		b.WriteByte(packets.PropResponseTopic)
		encoding.WriteString(b, []byte(p.ResponseTopic))
	}
	for _, v := range p.SubscriptionIdentifier {
		b.WriteByte(packets.PropSubscriptionIdentifier)
		l, _ := packets.DecodeRemainLength(int(v))
		b.Write(l)
	}
	for _, v := range p.UserProperties {
		b.WriteByte(packets.PropUser)
		encoding.WriteString(b, v.K)
		encoding.WriteString(b, v.V)
	}
	return
}

func (p *Publish) Decode(b *bytes.Buffer) (err error) {
	p.Message = &gmqtt.Message{}
	p.Dup, err = encoding.ReadBool(b)
	if err != nil {
		return err
	}
	p.QoS, err = b.ReadByte()
	if err != nil {
		return err
	}
	p.Retained, err = encoding.ReadBool(b)
	if err != nil {
		return err
	}
	topic, err := encoding.ReadString(b)
	if err != nil {
		return err
	}
	p.Topic = string(topic)
	p.Payload, err = encoding.ReadString(b)
	if err != nil {
		return err
	}
	p.PacketID, err = encoding.ReadUint16(b)
	if err != nil {
		return err
	}
	for {
		pt, err := b.ReadByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch pt {
		case packets.PropContentType:
			v, err := encoding.ReadString(b)
			if err != nil {
				return err
			}
			p.ContentType = string(v)
		case packets.PropCorrelationData:
			p.CorrelationData, err = encoding.ReadString(b)
			if err != nil {
				return err
			}
		case packets.PropMessageExpiry:
			p.MessageExpiry, err = encoding.ReadUint32(b)
			if err != nil {
				return err
			}
		case packets.PropPayloadFormat:
			p.PayloadFormat, err = b.ReadByte()
			if err != nil {
				return err
			}
		case packets.PropResponseTopic:
			v, err := encoding.ReadString(b)
			if err != nil {
				return err
			}
			p.ResponseTopic = string(v)
		case packets.PropSubscriptionIdentifier:
			si, err := packets.EncodeRemainLength(b)
			if err != nil {
				return err
			}
			p.SubscriptionIdentifier = append(p.SubscriptionIdentifier, uint32(si))
		case packets.PropUser:
			k, err := encoding.ReadString(b)
			if err != nil {
				return err
			}
			v, err := encoding.ReadString(b)
			if err != nil {
				return err
			}
			p.UserProperties = append(p.UserProperties, packets.UserProperty{K: k, V: v})
		}
	}
}

// Encode encode the pubrel structure into bytes.
func (p *Pubrel) Encode(b *bytes.Buffer) {
	encoding.WriteUint16(b, p.PacketID)
}

func (p *Pubrel) Decode(b *bytes.Buffer) (err error) {
	p.PacketID, err = encoding.ReadUint16(b)
	return
}

// Encode encode the elem structure into bytes.
// Format: 8 byte timestamp | 1 byte identifier| data
func (e *Elem) Encode() []byte {
	b := bytes.NewBuffer(make([]byte, 0, 100))
	rs := make([]byte, 19)
	binary.BigEndian.PutUint64(rs[0:9], uint64(e.At.Unix()))
	binary.BigEndian.PutUint64(rs[9:18], uint64(e.Expiry.Unix()))
	switch m := e.MessageWithID.(type) {
	case *Publish:
		rs[18] = 0
		b.Write(rs)
		m.Encode(b)
	case *Pubrel:
		rs[18] = 1
		b.Write(rs)
		m.Encode(b)
	}
	return b.Bytes()
}

func (e *Elem) Decode(b []byte) (err error) {
	if len(b) < 19 {
		return errors.New("invalid input length")
	}
	e.At = time.Unix(int64(binary.BigEndian.Uint64(b[0:9])), 0)
	e.Expiry = time.Unix(int64(binary.BigEndian.Uint64(b[9:19])), 0)
	switch b[18] {
	case 0: // publish
		p := &Publish{}
		buf := bytes.NewBuffer(b[19:])
		err = p.Decode(buf)
		e.MessageWithID = p
	case 1: // pubrel
		p := &Pubrel{}
		buf := bytes.NewBuffer(b[19:])
		err = p.Decode(buf)
		e.MessageWithID = p
	default:
		return errors.New("invalid identifier")
	}
	return
}
