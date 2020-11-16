package queue

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"time"

	"github.com/DrmagicE/gmqtt"
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
	writeBool(b, p.Dup)
	b.WriteByte(p.QoS)
	writeBool(b, p.Retained)
	writeString(b, []byte(p.Topic))
	writeString(b, []byte(p.Payload))
	writeUint16(b, p.PacketID)

	if len(p.ContentType) != 0 {
		b.WriteByte(packets.PropContentType)
		writeString(b, []byte(p.ContentType))
	}
	if len(p.CorrelationData) != 0 {
		b.WriteByte(packets.PropCorrelationData)
		writeString(b, []byte(p.CorrelationData))
	}
	if p.MessageExpiry != 0 {
		b.WriteByte(packets.PropMessageExpiry)
		writeUint32(b, p.MessageExpiry)
	}
	b.WriteByte(packets.PropPayloadFormat)
	b.WriteByte(p.PayloadFormat)

	if len(p.ResponseTopic) != 0 {
		b.WriteByte(packets.PropResponseTopic)
		writeString(b, []byte(p.ResponseTopic))
	}
	for _, v := range p.SubscriptionIdentifier {
		b.WriteByte(packets.PropSubscriptionIdentifier)
		l, _ := packets.DecodeRemainLength(int(v))
		b.Write(l)
	}
	for _, v := range p.UserProperties {
		b.WriteByte(packets.PropUser)
		writeString(b, v.K)
		writeString(b, v.V)
	}
	return
}

func (p *Publish) Decode(b *bytes.Buffer) (err error) {
	p.Message = &gmqtt.Message{}
	p.Dup, err = readBool(b)
	if err != nil {
		return err
	}
	p.QoS, err = b.ReadByte()
	if err != nil {
		return err
	}
	p.Retained, err = readBool(b)
	if err != nil {
		return err
	}
	topic, err := readString(b)
	if err != nil {
		return err
	}
	p.Topic = string(topic)
	p.Payload, err = readString(b)
	if err != nil {
		return err
	}
	p.PacketID, err = readUint16(b)
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
			v, err := readString(b)
			if err != nil {
				return err
			}
			p.ContentType = string(v)
		case packets.PropCorrelationData:
			p.CorrelationData, err = readString(b)
			if err != nil {
				return err
			}
		case packets.PropMessageExpiry:
			p.MessageExpiry, err = readUint32(b)
			if err != nil {
				return err
			}
		case packets.PropPayloadFormat:
			p.PayloadFormat, err = b.ReadByte()
			if err != nil {
				return err
			}
		case packets.PropResponseTopic:
			v, err := readString(b)
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
			k, err := readString(b)
			if err != nil {
				return err
			}
			v, err := readString(b)
			if err != nil {
				return err
			}
			p.UserProperties = append(p.UserProperties, packets.UserProperty{K: k, V: v})
		}
	}
}

// Encode encode the pubrel structure into bytes.
func (p *Pubrel) Encode(b *bytes.Buffer) {
	writeUint16(b, p.PacketID)
}

func (p *Pubrel) Decode(b *bytes.Buffer) (err error) {
	p.PacketID, err = readUint16(b)
	return
}

// Encode encode the elem structure into bytes.
// Format: 8 byte timestamp | 1 byte identifier| data
func (e *Elem) Encode() []byte {
	b := bytes.NewBuffer(make([]byte, 0, 100))
	rs := make([]byte, 9)
	binary.BigEndian.PutUint64(rs, uint64(e.At.Unix()))
	switch m := e.MessageWithID.(type) {
	case *Publish:
		rs[8] = 0
		b.Write(rs)
		m.Encode(b)
	case *Pubrel:
		rs[8] = 1
		b.Write(rs)
		m.Encode(b)
	}
	return b.Bytes()
}

func (e *Elem) Decode(b []byte) (err error) {
	if len(b) < 10 {
		return errors.New("invalid input length")
	}
	ts := b[0:8]
	e.At = time.Unix(int64(binary.BigEndian.Uint64(ts)), 0)
	switch b[8] {
	case 0: // publish
		p := &Publish{}
		buf := bytes.NewBuffer(b[9:])
		err = p.Decode(buf)
		e.MessageWithID = p
	case 1: // pubrel
		p := &Pubrel{}
		buf := bytes.NewBuffer(b[9:])
		err = p.Decode(buf)
		e.MessageWithID = p
	default:
		return errors.New("invalid identifier")
	}
	return
}

func readUint16(r *bytes.Buffer) (uint16, error) {
	if r.Len() < 2 {
		return 0, errors.New("invalid length")
	}
	return binary.BigEndian.Uint16(r.Next(2)), nil
}

func writeUint16(w *bytes.Buffer, i uint16) {
	w.WriteByte(byte(i >> 8))
	w.WriteByte(byte(i))
}

func writeBool(w *bytes.Buffer, b bool) {
	if b {
		w.WriteByte(1)
	} else {
		w.WriteByte(0)
	}
}

func readBool(r *bytes.Buffer) (bool, error) {
	b, err := r.ReadByte()
	if err != nil {
		return false, err
	}
	if b == 0 {
		return false, nil
	}
	return true, nil
}

func writeString(w *bytes.Buffer, s []byte) {
	writeUint16(w, uint16(len(s)))
	w.Write(s)
}
func readString(r *bytes.Buffer) (b []byte, err error) {
	l := make([]byte, 2)
	_, err = io.ReadFull(r, l)
	if err != nil {
		return nil, err
	}
	length := int(binary.BigEndian.Uint16(l))
	paylaod := make([]byte, length)

	_, err = io.ReadFull(r, paylaod)
	if err != nil {
		return nil, err
	}
	return paylaod, nil
}

func writeUint32(w *bytes.Buffer, i uint32) {
	w.WriteByte(byte(i >> 24))
	w.WriteByte(byte(i >> 16))
	w.WriteByte(byte(i >> 8))
	w.WriteByte(byte(i))
}

func readUint32(r *bytes.Buffer) (uint32, error) {
	if r.Len() < 4 {
		return 0, errors.New("invalid length")
	}
	return binary.BigEndian.Uint32(r.Next(4)), nil
}
