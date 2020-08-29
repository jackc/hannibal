package server_test

import (
	"testing"

	"github.com/jackc/hannibal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPGFuncHandler(t *testing.T) {
	// Success cases
	for _, tt := range []struct {
		desc        string
		name        string
		proargmodes []string
		proargnames []string
		sql         string
		inArgs      []string
	}{
		{
			desc:        "simple",
			name:        "get_foo",
			proargmodes: []string{"i", "o"},
			proargnames: []string{"args", "resp_body"},
			sql:         "select null as status, resp_body, null as template, null as template_data, null as cookie_session from get_foo(args => $1)",
			inArgs:      []string{"args"},
		},
		{
			desc:        "status",
			name:        "get_foo",
			proargmodes: []string{"i", "o", "o"},
			proargnames: []string{"args", "resp_body", "status"},
			sql:         "select status, resp_body, null as template, null as template_data, null as cookie_session from get_foo(args => $1)",
			inArgs:      []string{"args"},
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			h, err := server.NewPGFuncHandler(tt.name, tt.proargmodes, tt.proargnames)
			require.NoError(t, err)
			require.NotNil(t, h)
			assert.Equal(t, tt.sql, h.SQL)
			assert.Equal(t, tt.inArgs, h.FuncInArgs)
		})
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
			proargnames: []string{"args", "resp_body"},
			errString:   "name cannot be empty",
		},
		{
			desc:        "empty proargmodes",
			name:        "foo",
			proargmodes: []string{},
			proargnames: []string{"args", "resp_body"},
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
			proargnames: []string{"args", "resp_body", "bad"},
			errString:   "unknown arg: bad",
		},
		{
			desc:        "unknown out argument",
			name:        "foo",
			proargmodes: []string{"i", "o", "o"},
			proargnames: []string{"args", "resp_body", "bad"},
			errString:   "unknown arg: bad",
		},
		{
			desc:        "differing number of modes and names",
			name:        "foo",
			proargmodes: []string{"i", "o"},
			proargnames: []string{"args", "resp_body", "extra"},
			errString:   "proargmodes and proargnames are not the same length",
		},
		{
			desc:        "differing number of modes and names",
			name:        "foo",
			proargmodes: []string{"i", "o", "o"},
			proargnames: []string{"args", "resp_body"},
			errString:   "proargmodes and proargnames are not the same length",
		},
		{
			desc:        "missing status and resp_body",
			name:        "foo",
			proargmodes: []string{"i"},
			proargnames: []string{"args"},
			errString:   "missing status, resp_body, and template out arguments",
		},
		{
			desc:        "unknown proargmode",
			name:        "foo",
			proargmodes: []string{"i", "z"},
			proargnames: []string{"args", "resp_body"},
			errString:   "unknown proargmode: z",
		},
	} {
		h, err := server.NewPGFuncHandler(tt.name, tt.proargmodes, tt.proargnames)
		assert.EqualErrorf(t, err, tt.errString, "%d: %s", i, tt.desc)
		assert.Nilf(t, h, "%d: %s", i, tt.desc)
	}
}
