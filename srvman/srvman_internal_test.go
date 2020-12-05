package srvman

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterpolateColorVariables(t *testing.T) {
	// Success tests
	for _, tt := range []struct {
		desc string
		s    string
		args map[string]interface{}
		out  string
	}{
		{
			desc: "simple",
			s:    "http_server -p [[bluegreen.port]]",
			args: map[string]interface{}{"port": 1234},
			out:  "http_server -p 1234",
		},
		{
			desc: "multiple",
			s:    "http_server -d [[bluegreen.domain]] -p [[bluegreen.port]]",
			args: map[string]interface{}{"domain": "example.com", "port": 1234},
			out:  "http_server -d example.com -p 1234",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			result, err := interpolateColorVariables(tt.s, tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.out, result)
		})
	}

	// Fail cases
	for _, tt := range []struct {
		desc      string
		s         string
		args      map[string]interface{}
		errString string
	}{
		{
			desc:      "missing args",
			s:         "http_server -p [[bluegreen.port]]",
			args:      nil,
			errString: "missing key: port",
		},
		{
			desc:      "missing variable",
			s:         "http_server -p [[bluegreen.port]]",
			args:      map[string]interface{}{},
			errString: "missing key: port",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			result, err := interpolateColorVariables(tt.s, tt.args)
			assert.EqualError(t, err, tt.errString)
			assert.Equal(t, "", result)
		})
	}
}
