package config

type TopicAliasType = string

const (
	TopicAliasMgrTypeFIFO TopicAliasType = "fifo"
)

var (
	// DefaultTopicAliasManager is the default value of TopicAliasManager
	DefaultTopicAliasManager = TopicAliasManager{
		Type: TopicAliasMgrTypeFIFO,
	}
)

// TopicAliasManager is the config of the topic alias manager.
type TopicAliasManager struct {
	Type TopicAliasType
}
