package main

import (
	"github.com/DrmagicE/gmqtt/plugin/admin"
	"github.com/DrmagicE/gmqtt/plugin/prometheus"
	"github.com/DrmagicE/gmqtt/server"
)

var pluginOrder = []string{
	prometheus.Name,
	admin.Name,
}

func init() {
	server.SetPluginOrder(pluginOrder)
}
