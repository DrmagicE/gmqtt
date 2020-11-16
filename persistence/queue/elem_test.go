package queue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

func TestElem_Encode_Publish(t *testing.T) {
	a := assert.New(t)
	e := &Elem{
		At: time.Unix(time.Now().Unix(), 0),
		MessageWithID: &Publish{
			Message: &gmqtt.Message{
				Dup:                    false,
				QoS:                    2,
				Retained:               false,
				Topic:                  "/mytopic",
				Payload:                []byte("payload"),
				PacketID:               2,
				ContentType:            "type",
				CorrelationData:        nil,
				MessageExpiry:          1,
				PayloadFormat:          packets.PayloadFormatString,
				ResponseTopic:          "",
				SubscriptionIdentifier: []uint32{1, 2},
				UserProperties: []packets.UserProperty{
					{
						K: []byte("1"),
						V: []byte("2"),
					}, {
						K: []byte("3"),
						V: []byte("4"),
					},
				},
			},
		},
	}
	rs := e.Encode()
	de := &Elem{}
	err := de.Decode(rs)
	a.Nil(err)
	a.Equal(e, de)
}
func TestElem_Encode_Pubrel(t *testing.T) {
	a := assert.New(t)
	e := &Elem{
		At: time.Unix(time.Now().Unix(), 0),
		MessageWithID: &Pubrel{
			PacketID: 2,
		},
	}
	rs := e.Encode()
	de := &Elem{}
	err := de.Decode(rs)
	a.Nil(err)
	a.Equal(e, de)
}

func Benchmark_Encode_Publish(b *testing.B) {
	for i := 0; i < b.N; i++ {
		e := &Elem{
			At: time.Unix(time.Now().Unix(), 0),
			MessageWithID: &Publish{
				Message: &gmqtt.Message{
					Dup:                    false,
					QoS:                    2,
					Retained:               false,
					Topic:                  "/mytopic",
					Payload:                []byte("payload"),
					PacketID:               2,
					ContentType:            "type",
					CorrelationData:        nil,
					MessageExpiry:          1,
					PayloadFormat:          packets.PayloadFormatString,
					ResponseTopic:          "",
					SubscriptionIdentifier: []uint32{1, 2},
					UserProperties: []packets.UserProperty{
						{
							K: []byte("1"),
							V: []byte("2"),
						}, {
							K: []byte("3"),
							V: []byte("4"),
						},
					},
				},
			},
		}
		e.Encode()
	}
}
