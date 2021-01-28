// +build windows

package config

import (
	"os"
	"path/filepath"
)

func getDefaultPidFile() (string, error) {
	return filepath.Join(os.Getenv("programdata"), "gmqtt", "gmqttd.pid"), nil
}
