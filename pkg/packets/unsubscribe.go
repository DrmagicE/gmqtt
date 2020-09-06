package packets

import (
	"bytes"
	"fmt"
	"io"

	"github.com/DrmagicE/gmqtt/pkg/codes"
)

// Unsubscribe represents the MQTT Unsubscribe  packet.
type Unsubscribe struct {
	Version    Version
	FixHeader  *FixHeader
	PacketID   PacketID
	Topics     []string
	Properties *Properties
}

func (u *Unsubscribe) String() string {
	return fmt.Sprintf("Unsubscribe, Version: %v, Pid: %v, Topics: %v, Properties: %s", u.Version, u.PacketID, u.Topics, u.Properties)
}

// NewUnSubBack returns the Unsuback struct which is the ack packet of the Unsubscribe packet.
func (u *Unsubscribe) NewUnSubBack() *Unsuback {
	fh := &FixHeader{PacketType: UNSUBACK, Flags: 0}
	unSuback := &Unsuback{FixHeader: fh, PacketID: u.PacketID, Version: u.Version}
	if unSuback.Version == Version5 {
		unSuback.Payload = make([]codes.Code, len(u.Topics))
	}
	return unSuback
}

// NewUnsubscribePacket returns a Unsubscribe instance by the given FixHeader and io.Reader.
func NewUnsubscribePacket(fh *FixHeader, version Version, r io.Reader) (*Unsubscribe, error) {
	p := &Unsubscribe{FixHeader: fh, Version: version}
	//判断 标志位 flags 是否合法[MQTT-3.10.1-1]
	if fh.Flags != FlagUnsubscribe {
		return nil, codes.ErrMalformed
	}
	err := p.Unpack(r)
	if err != nil {
		return nil, err
	}
	return p, err
}

// Pack encodes the packet struct into bytes and writes it into io.Writer.
func (u *Unsubscribe) Pack(w io.Writer) error {
	u.FixHeader = &FixHeader{PacketType: UNSUBSCRIBE, Flags: FlagUnsubscribe}
	bufw := &bytes.Buffer{}
	writeUint16(bufw, u.PacketID)
	if u.Version == Version5 {
		u.Properties.Pack(bufw, UNSUBSCRIBE)
	}
	for _, topic := range u.Topics {
		writeUTF8String(bufw, []byte(topic))
	}
	u.FixHeader.RemainLength = bufw.Len()
	err := u.FixHeader.Pack(w)
	if err != nil {
		return err
	}
	_, err = bufw.WriteTo(w)
	return err
}

// Unpack read the packet bytes from io.Reader and decodes it into the packet struct.
func (u *Unsubscribe) Unpack(r io.Reader) error {
	restBuffer := make([]byte, u.FixHeader.RemainLength)
	_, err := io.ReadFull(r, restBuffer)
	if err != nil {
		return codes.ErrMalformed
	}
	bufr := bytes.NewBuffer(restBuffer)
	u.PacketID, err = readUint16(bufr)
	if err != nil {
		return err
	}

	if u.Version == Version5 {
		u.Properties = &Properties{}
		if err := u.Properties.Unpack(bufr, UNSUBSCRIBE); err != nil {
			return err
		}
	}
	for {
		topicFilter, err := readUTF8String(true, bufr)
		if err != nil {
			return err
		}
		if !ValidTopicFilter(true, topicFilter) {
			return codes.ErrProtocol
		}
		u.Topics = append(u.Topics, string(topicFilter))
		if bufr.Len() == 0 {
			return nil
		}
	}
}
