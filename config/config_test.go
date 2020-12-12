package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfig(t *testing.T) {
	var tt = []struct {
		caseName string
		fileName string
		hasErr   bool
		expected Config
	}{
		{
			caseName: "defaultConfig",
			fileName: "",
			hasErr:   false,
			expected: DefaultConfig(),
		},
	}

	for _, v := range tt {
		t.Run(v.caseName, func(t *testing.T) {
			a := assert.New(t)
			c, err := ParseConfig(v.fileName)
			if v.hasErr {
				a.NotNil(err)
			} else {
				a.Nil(err)
			}
			a.Equal(v.expected, c)
		})
	}
}
