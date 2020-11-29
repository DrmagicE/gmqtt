// +build !windows,!darwin

package config

func getDefaultPidFile() string {
	return "/var/run/gmqttd.pid"
}
