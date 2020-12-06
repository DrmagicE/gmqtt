package main

import (
	"github.com/DrmagicE/gmqtt/plugin/prometheus"
	"github.com/DrmagicE/gmqtt/server"
)

var pluginOrder = []string{
	prometheus.Name,
}

func init() {
	server.SetPluginOrder(pluginOrder)
}
