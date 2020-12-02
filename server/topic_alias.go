package server

import (
	"github.com/DrmagicE/gmqtt/config"
	"github.com/DrmagicE/gmqtt/pkg/packets"
)

type NewTopicAliasManager func(config config.Config, maxAlias uint16, clientID string) TopicAliasManager

// TopicAliasManager manage the topic alias for a V5 client.
// see topicalias/fifo for more details.
type TopicAliasManager interface {
	// Check return the alias number and whether the alias exist.
	// For examples:
	// If the Publish alias exist and the manager decides to use the alias, it return the alias number and true.
	// If the Publish alias exist, but the manager decides not to use alias, it return 0 and true.
	// If the Publish alias not exist and the manager decides to assign a new alias, it return the new alias and false.
	// If the Publish alias not exist, but the manager decides not to assign alias, it return the 0 and false.
	Check(publish *packets.Publish) (alias uint16, exist bool)
}
