package admin

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/status"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"github.com/DrmagicE/gmqtt/server"
)

func TestPublisher_Publish(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mp := server.NewMockPublisher(ctrl)
	pub := &publisher{
		a: &Admin{
			publisher: mp,
		},
	}
	msg := &gmqtt.Message{
		QoS:             1,
		Retained:        true,
		Topic:           "topic",
		Payload:         []byte("abc"),
		ContentType:     "ct",
		CorrelationData: []byte("co"),
		MessageExpiry:   1,
		PayloadFormat:   1,
		ResponseTopic:   "resp",
		UserProperties: []packets.UserProperty{
			{
				K: []byte("K"),
				V: []byte("V"),
			},
		},
	}
	mp.EXPECT().Publish(msg)
	_, err := pub.Publish(context.Background(), &PublishRequest{
		TopicName:       msg.Topic,
		Payload:         string(msg.Payload),
		Qos:             uint32(msg.QoS),
		Retained:        msg.Retained,
		ContentType:     msg.ContentType,
		CorrelationData: string(msg.CorrelationData),
		MessageExpiry:   msg.MessageExpiry,
		PayloadFormat:   uint32(msg.PayloadFormat),
		ResponseTopic:   msg.ResponseTopic,
		UserProperties: []*UserProperties{
			{
				K: []byte("K"),
				V: []byte("V"),
			},
		},
	})
	a.Nil(err)
}

func TestPublisher_Publish_InvalidArgument(t *testing.T) {
	var tt = []struct {
		name  string
		field string
		req   *PublishRequest
	}{
		{
			name:  "invalid_topic_name",
			field: "topic_name",
			req: &PublishRequest{
				TopicName: "/a/b/+",
				Qos:       2,
			},
		},
		{
			name:  "invalid_qos",
			field: "qos",
			req: &PublishRequest{
				TopicName: "a",
				Qos:       3,
			},
		},
		{
			name:  "invalid_payload_format",
			field: "payload_format",
			req: &PublishRequest{
				TopicName:     "a",
				Qos:           2,
				PayloadFormat: 3,
			},
		},
		{
			name:  "invalid_response_topic",
			field: "response_topic",
			req: &PublishRequest{
				TopicName:     "a",
				Qos:           2,
				PayloadFormat: 1,
				ResponseTopic: "#/",
			},
		},
	}
	for _, v := range tt {
		t.Run(v.name, func(t *testing.T) {
			a := assert.New(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mp := server.NewMockPublisher(ctrl)
			pub := &publisher{
				a: &Admin{
					publisher: mp,
				},
			}
			_, err := pub.Publish(context.Background(), v.req)
			s, ok := status.FromError(err)
			a.True(ok)
			a.Contains(s.Message(), v.field)
		})
	}

}
