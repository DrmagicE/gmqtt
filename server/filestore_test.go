package server

import (
	"bytes"
	"github.com/DrmagicE/gmqtt/pkg/packets"
	"testing"
	"time"
)

var testPutGetOfflineMsg = []struct {
	clientId string
	p        []*packets.Publish
}{
	{clientId: "clientA",
		p: []*packets.Publish{{
			Dup:       false,
			Qos:       packets.QOS_1,
			Retain:    false,
			TopicName: []byte("topicA"),
			PacketId:  1,
			Payload:   []byte("payloadA"),
		}, {
			Dup:       false,
			Qos:       packets.QOS_2,
			Retain:    false,
			TopicName: []byte("topicB"),
			PacketId:  2,
			Payload:   []byte("payloadB"),
		}}},

	{clientId: "clientB",
		p: []*packets.Publish{{
			Dup:       false,
			Qos:       packets.QOS_0,
			Retain:    false,
			TopicName: []byte("topicAA"),
			PacketId:  0,
			Payload:   []byte("payloadAA"),
		}, {
			Dup:       false,
			Qos:       packets.QOS_2,
			Retain:    false,
			TopicName: []byte("topicBB"),
			PacketId:  2,
			Payload:   []byte("payloadBB"),
		}}},
}

var testPutGetSessions = []*SessionPersistence{
	{
		ClientId:  "cA",
		SubTopics: map[string]packets.Topic{"tA": {Name: "tA", Qos: packets.QOS_0}, "tB": {Name: "tB", Qos: packets.QOS_1}, "tC": {Name: "tC", Qos: packets.QOS_2}},
		Inflight: []*InflightElem{
			{At: time.Date(2018, 1, 1, 1, 1, 1, 1, time.Local), Pid: 1, Packet: &packets.Publish{}},
			{At: time.Date(2018, 1, 1, 1, 1, 1, 2, time.Local), Pid: 2, Packet: &packets.Pubrel{}},
		},
		UnackPublish: map[packets.PacketId]bool{1: true, 2: true},
		Pid:          map[packets.PacketId]bool{1: true, 2: true},
	},
	{
		ClientId:  "cB",
		SubTopics: map[string]packets.Topic{"tb": {Name: "tA", Qos: packets.QOS_0}, "tB": {Name: "tB", Qos: packets.QOS_1}, "tC": {Name: "tC", Qos: packets.QOS_2}},
		Inflight: []*InflightElem{
			{At: time.Date(2018, 1, 1, 1, 1, 1, 1, time.Local), Pid: 1, Packet: &packets.Publish{}},
			{At: time.Date(2018, 1, 1, 1, 1, 1, 2, time.Local), Pid: 2, Packet: &packets.Pubrel{}},
			{At: time.Date(2018, 1, 1, 1, 1, 1, 3, time.Local), Pid: 3, Packet: &packets.Publish{}},
		},
		UnackPublish: map[packets.PacketId]bool{1: true, 2: true, 3: true},
		Pid:          map[packets.PacketId]bool{1: true, 2: true, 3: true},
	},
}

func TestFileStore_PutAndGetOfflineMsg(t *testing.T) {
	var err error
	f := &FileStore{Path: "testdata/offlineMsg"}
	err = f.Open()
	defer func() {
		err := f.Close()
		if err != nil {
			t.Fatalf("unexpected err: %s", err)
		}
	}()
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	for _, v := range testPutGetOfflineMsg {
		tt := make([]packets.Packet, 0, len(v.p))
		b := &bytes.Buffer{}
		for _, p := range v.p {
			err := p.Pack(b)
			if err != nil {
				t.Fatalf("unexpected err: %s", err)
			}
			tt = append(tt, p)
		}
		err = f.PutOfflineMsg(v.clientId, tt)
		if err != nil {
			t.Fatalf("PutOfflineMsg(%s) error: %s", v.clientId, err)
		}
	}

	for _, v := range testPutGetOfflineMsg {
		pp, err := f.GetOfflineMsg(v.clientId)

		if err != nil {
			t.Fatalf("GetOfflineMsg(%s) error: %s", v.clientId, err)
		}
		for k, p := range pp {
			if p.String() != v.p[k].String() {
				t.Fatalf("GetOfflineMsg(%s) error, want %s, got %s", v.clientId, v.p[k].String(), p.String())
			}
		}
	}


}

func TestFileStore_PutAndGetSessions(t *testing.T) {
	//defer os.RemoveAll("testdata/session")
	var err error
	f := &FileStore{Path: "testdata/session"}
	err = f.Open()
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	err = f.PutSessions(testPutGetSessions)
	if err != nil {
		t.Fatalf("GetOfflineMsg() error:%s", err)
	}
	err = f.Close()
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	ff := &FileStore{Path: "testdata/session"}
	err = ff.Open()
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	sp, err := ff.GetSessions()
	err = ff.Close()
	if err != nil {
		t.Fatalf("unexpected err: %s", err)
	}
	if err != nil {
		t.Fatalf("GetSessions() error:%s", err)
	}
	if len(sp) == 0 {
		t.Fatalf("len error,want %d, got %d",len(testPutGetSessions),0 )
	}
	for k, v := range sp {
		if v.ClientId != testPutGetSessions[k].ClientId {
			t.Fatalf("ClientId error,want %s, got %s",testPutGetSessions[k].ClientId, v.ClientId )
		}
	}

}
