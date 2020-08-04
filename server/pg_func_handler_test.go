package server_test

import (
	"testing"

	"github.com/jackc/hannibal/server"
	"github.com/stretchr/testify/assert"
)

func TestNewPGFuncHandler(t *testing.T) {
	// Success cases
	for i, tt := range []struct {
		desc        string
		proargmodes []string
		proargnames []string
		inArgs      []string
		outArgs     []string
	}{
		{
			desc:        "simple",
			proargmodes: []string{"i", "o"},
			proargnames: []string{"query_args", "resp_body"},
			inArgs:      []string{"query_args"},
			outArgs:     []string{"resp_body"},
		},
		{
			desc:        "status comes first",
			proargmodes: []string{"i", "o", "o"},
			proargnames: []string{"query_args", "resp_body", "status"},
			inArgs:      []string{"query_args"},
			outArgs:     []string{"status", "resp_body"},
		},
	} {
		h, err := server.NewPGFuncHandler("foo", tt.proargmodes, tt.proargnames)
		if assert.NoErrorf(t, err, "%d: %s", i, tt.desc) {
			assert.NotNilf(t, h, "%d: %s", i, tt.desc)
			assert.Equalf(t, "foo", h.Name, "%d: %s", i, tt.desc)
			assert.Equalf(t, tt.inArgs, h.InArgs, "%d: %s", i, tt.desc)
			assert.Equalf(t, tt.outArgs, h.OutArgs, "%d: %s", i, tt.desc)
		}
	}

	// Fail cases
	for i, tt := range []struct {
		desc        string
		name        string
		proargmodes []string
		proargnames []string
		errString   string
	}{
		{
			desc:        "empty name",
			name:        "",
			proargmodes: []string{"i", "o"},
			proargnames: []string{"query_args", "resp_body"},
			errString:   "name cannot be empty",
		},
		{
			desc:        "empty proargmodes",
			name:        "foo",
			proargmodes: []string{},
			proargnames: []string{"query_args", "resp_body"},
			errString:   "proargmodes cannot be empty",
		},
		{
			desc:        "empty proargnames",
			name:        "foo",
			proargmodes: []string{"i", "o"},
			proargnames: []string{},
			errString:   "proargnames cannot be empty",
		},
		{
			desc:        "unknown in argument",
			name:        "foo",
			proargmodes: []string{"i", "o", "i"},
			proargnames: []string{"query_args", "resp_body", "bad"},
			errString:   "unknown arg: bad",
		},
		{
			desc:        "unknown out argument",
			name:        "foo",
			proargmodes: []string{"i", "o", "o"},
			proargnames: []string{"query_args", "resp_body", "bad"},
			errString:   "unknown arg: bad",
		},
		{
			desc:        "differing number of modes and names",
			name:        "foo",
			proargmodes: []string{"i", "o"},
			proargnames: []string{"query_args", "resp_body", "extra"},
			errString:   "proargmodes and proargnames are not the same length",
		},
		{
			desc:        "differing number of modes and names",
			name:        "foo",
			proargmodes: []string{"i", "o", "o"},
			proargnames: []string{"query_args", "resp_body"},
			errString:   "proargmodes and proargnames are not the same length",
		},
		{
			desc:        "missing status and resp_body",
			name:        "foo",
			proargmodes: []string{"i"},
			proargnames: []string{"query_args"},
			errString:   "missing status and resp_body args",
		},
		{
			desc:        "unknown proargmode",
			name:        "foo",
			proargmodes: []string{"i", "z"},
			proargnames: []string{"query_args", "resp_body"},
			errString:   "unknown proargmode: z",
		},
	} {
		h, err := server.NewPGFuncHandler(tt.name, tt.proargmodes, tt.proargnames)
		assert.EqualErrorf(t, err, tt.errString, "%d: %s", i, tt.desc)
		assert.Nilf(t, h, "%d: %s", i, tt.desc)
	}
}
