// +build !windows

package config

func getDefaultPidFile() (string, error) {
	return "/var/run/gmqttd.pid", nil
}
