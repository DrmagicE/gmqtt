package packets

// Topic represents the MQTT Topic
type Topic struct {
	SubOptions
	Name string
}
type SubOptions struct {
	Qos               uint8
	RetainHandling    byte
	NoLocal           bool
	RetainAsPublished bool
}
