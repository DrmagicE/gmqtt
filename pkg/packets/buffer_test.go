package packets

import (
	"bytes"
	"testing"
)

func TestBufferPoolWithPacketPack(t *testing.T) {
	// Test that buffer pool works correctly with actual packet packing
	testCases := []struct {
		name   string
		packet Packet
	}{
		{
			name: "Connect",
			packet: &Connect{
				Version:       Version5,
				ProtocolName:  []byte("MQTT"),
				ProtocolLevel: 5,
				CleanStart:    true,
				KeepAlive:     60,
				ClientID:      []byte("test-client"),
			},
		},
		{
			name: "Publish",
			packet: &Publish{
				Version:   Version5,
				Dup:       false,
				Qos:       Qos1,
				Retain:    false,
				TopicName: []byte("test/topic"),
				PacketID:  1,
				Payload:   []byte("test payload"),
			},
		},
		{
			name: "Subscribe",
			packet: &Subscribe{
				Version:  Version5,
				PacketID: 1,
				Topics: []Topic{
					{Name: "test/topic", SubOptions: SubOptions{Qos: Qos1}},
				},
			},
		},
		{
			name: "Puback",
			packet: &Puback{
				Version:  Version5,
				PacketID: 1,
			},
		},
		{
			name: "Connack",
			packet: &Connack{
				Version: Version5,
				Code:    0,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := tc.packet.Pack(&buf)
			if err != nil {
				t.Fatalf("Pack failed: %v", err)
			}
			if buf.Len() == 0 {
				t.Fatal("packed packet has zero length")
			}
		})
	}
}

func BenchmarkBufferPool(b *testing.B) {
	b.RunParallel(func(b *testing.PB) {
		for b.Next() {
			buf := getBuffer()
			buf.Write([]byte("benchmark test data that is somewhat realistic"))
			putBuffer(buf)
		}
	})
}

func BenchmarkBufferAlloc(b *testing.B) {
	for b.N > 0 {
		buf := &bytes.Buffer{}
		buf.Write([]byte("benchmark test data that is somewhat realistic"))
	}
}

func BenchmarkPublishPackWithPool(b *testing.B) {
	packet := &Publish{
		Version:   Version5,
		Dup:       false,
		Qos:       Qos1,
		Retain:    false,
		TopicName: []byte("test/topic/with/some/multi/level/structure"),
		PacketID:  1,
		Payload:   []byte("test payload data that is somewhat realistic for benchmarking"),
		Properties: &Properties{
			ContentType: []byte("application/json"),
		},
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var buf bytes.Buffer
		for pb.Next() {
			buf.Reset()
			packet.Pack(&buf)
		}
	})
}
