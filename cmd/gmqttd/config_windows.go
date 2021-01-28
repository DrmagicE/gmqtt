// +build windows

package main

import (
	"os"
	"path/filepath"
)

func getDefaultConfigDir() (string, error) {
	return filepath.Join(os.Getenv("programdata"), "gmqtt"), nil
}
