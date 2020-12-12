package queue

import (
	"bytes"
	"encoding/binary"
	"errors"
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
	encoding.EncodeMessage(p.Message, b)
}

func (p *Publish) Decode(b *bytes.Buffer) (err error) {
	msg, err := encoding.DecodeMessage(b)
	if err != nil {
		return err
	}
	p.Message = msg
	return nil
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
