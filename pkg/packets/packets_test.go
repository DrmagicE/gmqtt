package packets

import (
	"bufio"
	"bytes"
	"testing"
)

func appendPacket(firstByte byte, b ...[]byte) []byte {
	var rs []byte
	var l int
	for _, v := range b {
		l += len(v)
	}
	lb, _ := DecodeRemainLength(l)
	rs = append(rs, firstByte)
	rs = append(rs, lb...)
	for _, v := range b {
		rs = append(rs, v...)
	}
	return rs

}
func uint32P(v uint32) *uint32 {
	return &v
}
func uint16P(v uint16) *uint16 {
	return &v
}
func byteP(v byte) *byte {
	return &v
}

var testDecodeUTF8String = []struct {
	buf       []byte
	wantBytes []byte
	wantSize  int
	wantErr   error
}{
	{buf: []byte{0, 2, 0x31, 0x32, 0x33}, wantBytes: []byte{0x31, 0x32}, wantSize: 4, wantErr: nil},
	{buf: []byte{0, 2, 0x31, 0x32}, wantBytes: []byte{0x31, 0x32}, wantSize: 4, wantErr: nil},
	{buf: []byte{0, 2, 0x31}, wantBytes: nil, wantSize: 0, wantErr: ErrInvalUTF8String},
	{buf: []byte{0, 2, 0x01}, wantBytes: nil, wantSize: 0, wantErr: ErrInvalUTF8String},
	{buf: []byte{0, 2, 0, 0}, wantBytes: nil, wantSize: 0, wantErr: ErrInvalUTF8String},
}

var testEncodeUTF8String = []struct {
	buf       []byte
	wantBytes []byte
	wantSize  int
	wantErr   error
}{
	{buf: []byte{0x31, 0x32, 0x33}, wantBytes: []byte{0, 3, 0x31, 0x32, 0x33}, wantSize: 5, wantErr: nil},
	{buf: []byte{0x31}, wantBytes: []byte{0, 1, 0x31}, wantSize: 3, wantErr: nil},
	{buf: []byte{}, wantBytes: []byte{0, 0}, wantSize: 2, wantErr: nil},
}

func TestEncodeUTF8String(t *testing.T) {
	for _, v := range testEncodeUTF8String {
		b, size, err := EncodeUTF8String(v.buf)
		if !bytes.Equal(b, v.wantBytes) {
			t.Errorf("EncodeUTF8String(%v) error, want %v, but %v", v.buf, v.wantBytes, b)
		}
		if size != v.wantSize {
			t.Errorf("EncodeUTF8String(%v) error, want %d, but %d", v.buf, v.wantSize, size)
		}
		if err != v.wantErr {
			t.Errorf("EncodeUTF8String(%v) error, want %v, but %v", v.buf, v.wantErr, err)
		}
	}
}

func TestDecodeUTF8String(t *testing.T) {
	for _, v := range testDecodeUTF8String {
		b, size, err := DecodeUTF8String(v.buf)
		if !bytes.Equal(b, v.wantBytes) {
			t.Errorf("DecodeUTF8String(%v) error, want %v, but %v", v.buf, v.wantBytes, b)
		}
		if size != v.wantSize {
			t.Errorf("DecodeUTF8String(%v) error, want %d, but %d", v.buf, v.wantSize, size)
		}
		if err != v.wantErr {
			t.Errorf("DecodeUTF8String(%v) error, want %v, but %v", v.buf, v.wantErr, err)
		}
	}
}

var topicFilterTest = []struct {
	input string
	want  bool
}{
	{input: "sport/tennis#", want: false},
	{input: "sport/tennis/#/rank", want: false},
	{input: "//1", want: true},
	{input: "/+1", want: false},
	{input: "+", want: true},
	{input: "#", want: true},
	{input: "sport/tennis/#", want: true},
	{input: "/1/+/#", want: true},
	{input: "/1/+/+/1234", want: true},
	{input: "##", want: false},
	{input: "#/", want: false},
}

var sharedTopicFilterTest = []struct {
	input string
	want  bool
}{
	{input: "$share/sport/tennis#", want: false},
	{input: "$share/sport/tennis/#/rank", want: false},
	{input: "$share//1", want: false},
	{input: "$share/+1", want: false},
	{input: "$share/a/", want: false},
	{input: "$share/a/+", want: true},
	{input: "$share/a/#", want: true},
	{input: "$share/1/+/#", want: true},
	{input: "$share/+/1", want: false},
	{input: "$share/#/2", want: false},
	{input: "$share/a/b", want: true},
}

var topicNameTest = []struct {
	input string
	want  bool
}{
	{input: "sport/tennis#", want: false},
	{input: "sport/tennis/#/rank", want: false},
	{input: "//1", want: true},
	{input: "/+1", want: false},
	{input: "+", want: false},
	{input: "#", want: false},
	{input: "sport/tennis/#", want: false},
	{input: "/1/+/#", want: false},
	{input: "/1/+/+/1234", want: false},
	{input: "/abc/def/gggggg/", want: true},
	{input: "/9 2", want: true},
}

var topicMatchTest = []struct {
	subTopic string //subscribe topic
	topic    string //publish topic
	isMatch  bool
}{
	{subTopic: "#", topic: "/abc/def", isMatch: true},
	{subTopic: "/a", topic: "a", isMatch: false},
	{subTopic: "+", topic: "/a", isMatch: false},

	{subTopic: "a/", topic: "a", isMatch: false},
	{subTopic: "a/+", topic: "a/123/4", isMatch: false},
	{subTopic: "a/#", topic: "a/123/4", isMatch: true},

	{subTopic: "/a/+/+/abcd", topic: "/a/dfdf/3434/abcd", isMatch: true},
	{subTopic: "/a/+/+/abcd", topic: "/a/dfdf/3434/abcdd", isMatch: false},
	{subTopic: "/a/+/abc/", topic: "/a/dfdf/abc/", isMatch: true},
	{subTopic: "/a/+/abc/", topic: "/a/dfdf/abc", isMatch: false},
	{subTopic: "/a/+/+/", topic: "/a/dfdf/", isMatch: false},
	{subTopic: "/a/+/+", topic: "/a/dfdf/", isMatch: true},
	{subTopic: "/a/+/+/#", topic: "/a/dfdf/", isMatch: true},
}

func TestRemainLengthEncodeDecode(t *testing.T) {

	lengths := map[int][]byte{
		0:         {0x00},
		127:       {0x7F},
		128:       {0x80, 0x01},
		16383:     {0xFF, 0x7F},
		16384:     {0x80, 0x80, 0x01},
		2097151:   {0xFF, 0xFF, 0x7F},
		2097152:   {0x80, 0x80, 0x80, 0x01},
		268435455: {0xFF, 0xFF, 0xFF, 0x7F},
	}
	for k, v := range lengths {
		result, err := DecodeRemainLength(k)
		if err != nil {
			t.Fatalf("DecodeRemainLength(%v) error:%s", k, err)
		}
		if !bytes.Equal(v, result) {
			t.Fatalf("DecodeRemainLength(%v) error, want %d, but %d", k, v, result)
		}

		reader := bytes.NewReader(v)
		bufrd := bufio.NewReaderSize(reader, len(v))
		encodedLen, err := EncodeRemainLength(bufrd)
		if err != nil {
			t.Fatalf("EncodeRemainLength %v unexpected error:", err)
		}
		if encodedLen != k {
			t.Fatalf("EncodeRemainLength %v error, want %d, but %d", v, k, encodedLen)
		}
	}
}

func TestValidUTF8(t *testing.T) {
	runeByte := make([]byte, 1)
	for i := 0x00; i <= 0x1f; i++ { //\u0000 - \u001f invalid
		runeByte[0] = byte(i)
		if ValidUTF8(runeByte) == true {
			t.Fatalf("ValidUTF8(%v) error,want %t, but %t", runeByte, false, true)
		}
	}
	for i := 0x7F; i <= 0x9f; i++ { //\u007f - \u009f invalid
		runeByte[0] = byte(i)
		if ValidUTF8(runeByte) == true {
			t.Fatalf("ValidUTF8(%v) error,want %t, but %t", runeByte, false, true)
		}
	}
}

//Subscribable Topic Filter
func TestValidTopicFilter(t *testing.T) {
	for _, v := range topicFilterTest {
		if valid := ValidTopicFilter(true, []byte(v.input)); valid != v.want {
			t.Fatalf("ValidTopicFilter(%v) error,want %t, but %t", v.input, v.want, valid)
		}
	}
}

//Subscribable Topic Filter
func TestValidV5TopicFilter(t *testing.T) {
	for _, v := range sharedTopicFilterTest {
		if valid := ValidV5Topic([]byte(v.input)); valid != v.want {
			t.Fatalf("ValidTopicFilter(%v) error,want %t, but %t", v.input, v.want, valid)
		}
	}
}

func TestValidTopicName(t *testing.T) {
	for _, v := range topicNameTest {
		if valid := ValidTopicName(true, []byte(v.input)); valid != v.want {
			t.Fatalf("ValidTopicName(%v) error,want %t, but %t", v.input, v.want, valid)
		}
	}
}

func TestTopicMatch(t *testing.T) {
	for _, v := range topicMatchTest {
		if isMatch := TopicMatch([]byte(v.topic), []byte(v.subTopic)); isMatch != v.isMatch {
			t.Fatalf("TopicMatch(%s,%s) error,want %t, but %t", v.topic, v.subTopic, v.isMatch, isMatch)
		}
	}
}
