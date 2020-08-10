package main

import (
	"fmt"

	"github.com/DrmagicE/gmqtt"
	"github.com/DrmagicE/gmqtt/retained/trie"
)

func main() {

	t := trie.NewStore()
	t.AddOrReplace(gmqtt.NewMessage("TopicA", []byte("abc"), 2))
	fmt.Println(t.GetMatchedMessages("TopicA"))

}
