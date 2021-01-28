// +build !windows

package main

var (
	DefaultConfigDir = "/etc/gmqtt"
)

func getDefaultConfigDir() (string, error) {
	return DefaultConfigDir, nil
}
