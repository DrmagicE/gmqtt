package encoding

import (
	"bytes"
	"encoding/binary"
	"io"
	"time"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

// EncodeMessage encodes message into bytes and write it to the buffer
func EncodeMessage(msg *gmqtt.Message, b *bytes.Buffer) {
	if msg == nil {
		return
	}
	WriteBool(b, msg.Dup)
	b.WriteByte(msg.QoS)
	WriteBool(b, msg.Retained)
	WriteString(b, []byte(msg.Topic))
	WriteString(b, []byte(msg.Payload))
	WriteUint16(b, msg.PacketID)

	if len(msg.ContentType) != 0 {
		b.WriteByte(packets.PropContentType)
		WriteString(b, []byte(msg.ContentType))
	}
	if len(msg.CorrelationData) != 0 {
		b.WriteByte(packets.PropCorrelationData)
		WriteString(b, []byte(msg.CorrelationData))
	}
	if msg.MessageExpiry != 0 {
		b.WriteByte(packets.PropMessageExpiry)
		WriteUint32(b, msg.MessageExpiry)
	}
	b.WriteByte(packets.PropPayloadFormat)
	b.WriteByte(msg.PayloadFormat)

	if len(msg.ResponseTopic) != 0 {
		b.WriteByte(packets.PropResponseTopic)
		WriteString(b, []byte(msg.ResponseTopic))
	}
	for _, v := range msg.SubscriptionIdentifier {
		b.WriteByte(packets.PropSubscriptionIdentifier)
		l, _ := packets.DecodeRemainLength(int(v))
		b.Write(l)
	}
	for _, v := range msg.UserProperties {
		b.WriteByte(packets.PropUser)
		WriteString(b, v.K)
		WriteString(b, v.V)
	}
	return
}

// DecodeMessage decodes message from buffer.
func DecodeMessage(b *bytes.Buffer) (msg *gmqtt.Message, err error) {
	msg = &gmqtt.Message{}
	msg.Dup, err = ReadBool(b)
	if err != nil {
		return
	}
	msg.QoS, err = b.ReadByte()
	if err != nil {
		return
	}
	msg.Retained, err = ReadBool(b)
	if err != nil {
		return
	}
	topic, err := ReadString(b)
	if err != nil {
		return
	}
	msg.Topic = string(topic)
	msg.Payload, err = ReadString(b)
	if err != nil {
		return
	}
	msg.PacketID, err = ReadUint16(b)
	if err != nil {
		return
	}
	for {
		pt, err := b.ReadByte()
		if err == io.EOF {
			return msg, nil
		}
		if err != nil {
			return nil, err
		}
		switch pt {
		case packets.PropContentType:
			v, err := ReadString(b)
			if err != nil {
				return nil, err
			}
			msg.ContentType = string(v)
		case packets.PropCorrelationData:
			msg.CorrelationData, err = ReadString(b)
			if err != nil {
				return nil, err
			}
		case packets.PropMessageExpiry:
			msg.MessageExpiry, err = ReadUint32(b)
			if err != nil {
				return nil, err
			}
		case packets.PropPayloadFormat:
			msg.PayloadFormat, err = b.ReadByte()
			if err != nil {
				return nil, err
			}
		case packets.PropResponseTopic:
			v, err := ReadString(b)
			if err != nil {
				return nil, err
			}
			msg.ResponseTopic = string(v)
		case packets.PropSubscriptionIdentifier:
			si, err := packets.EncodeRemainLength(b)
			if err != nil {
				return nil, err
			}
			msg.SubscriptionIdentifier = append(msg.SubscriptionIdentifier, uint32(si))
		case packets.PropUser:
			k, err := ReadString(b)
			if err != nil {
				return nil, err
			}
			v, err := ReadString(b)
			if err != nil {
				return nil, err
			}
			msg.UserProperties = append(msg.UserProperties, packets.UserProperty{K: k, V: v})
		}
	}
}

// DecodeMessageFromBytes decodes message from bytes.
func DecodeMessageFromBytes(b []byte) (msg *gmqtt.Message, err error) {
	if len(b) == 0 {
		return nil, nil
	}
	return DecodeMessage(bytes.NewBuffer(b))
}

func EncodeSession(sess *gmqtt.Session, b *bytes.Buffer) {
	WriteString(b, []byte(sess.ClientID))
	if sess.Will != nil {
		b.WriteByte(1)
		EncodeMessage(sess.Will, b)
		WriteUint32(b, sess.WillDelayInterval)
	} else {
		b.WriteByte(0)
	}
	time := make([]byte, 8)
	binary.BigEndian.PutUint64(time, uint64(sess.ConnectedAt.Unix()))
	WriteUint32(b, sess.ExpiryInterval)
}

func DecodeSession(b *bytes.Buffer) (sess *gmqtt.Session, err error) {
	sess = &gmqtt.Session{}
	cid, err := ReadString(b)
	if err != nil {
		return nil, err
	}
	sess.ClientID = string(cid)
	willPresent, err := b.ReadByte()
	if err != nil {
		return
	}
	if willPresent == 1 {
		sess.Will, err = DecodeMessage(b)
		if err != nil {
			return
		}
		sess.WillDelayInterval, err = ReadUint32(b)
		if err != nil {
			return
		}
	}
	t := binary.BigEndian.Uint64(b.Next(8))
	sess.ConnectedAt = time.Unix(int64(t), 0)
	sess.ExpiryInterval, err = ReadUint32(b)
	return
}
