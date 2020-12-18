package main

import (
	"github.com/DrmagicE/gmqtt/plugin/admin"
	"github.com/DrmagicE/gmqtt/plugin/prometheus1"
	"github.com/DrmagicE/gmqtt/server"
)

var pluginOrder = []string{
	prometheus1.Name,
	admin.Name,
}

func init() {
	server.SetPluginOrder(pluginOrder)
}
