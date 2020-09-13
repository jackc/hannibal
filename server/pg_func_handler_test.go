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
		desc      string
		name      string
		inArgMap  map[string]struct{}
		outArgMap map[string]struct{}
		sql       string
		inArgs    []string
	}{
		{
			desc:      "simple",
			name:      "get_foo",
			inArgMap:  map[string]struct{}{"args": {}},
			outArgMap: map[string]struct{}{"resp_body": {}},
			sql:       "select null as status, resp_body, null as template, null as template_data, null as cookie_session from get_foo(args => $1)",
			inArgs:    []string{"args"},
		},
		{
			desc:      "status",
			name:      "get_foo",
			inArgMap:  map[string]struct{}{"args": {}},
			outArgMap: map[string]struct{}{"resp_body": {}, "status": {}},
			sql:       "select status, resp_body, null as template, null as template_data, null as cookie_session from get_foo(args => $1)",
			inArgs:    []string{"args"},
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			h, err := server.NewPGFuncHandler(tt.name, tt.inArgMap, tt.outArgMap)
			require.NoError(t, err)
			require.NotNil(t, h)
			assert.Equal(t, tt.sql, h.SQL)
			assert.Equal(t, tt.inArgs, h.FuncInArgs)
		})
	}

	// Fail cases
	for i, tt := range []struct {
		desc      string
		name      string
		inArgMap  map[string]struct{}
		outArgMap map[string]struct{}
		errString string
	}{
		{
			desc:      "empty name",
			name:      "",
			inArgMap:  map[string]struct{}{"args": {}},
			outArgMap: map[string]struct{}{"resp_body": {}},
			errString: "name cannot be empty",
		},
		{
			desc:      "unknown in argument",
			name:      "foo",
			inArgMap:  map[string]struct{}{"args": {}, "bad": {}},
			outArgMap: map[string]struct{}{"resp_body": {}},
			errString: "unknown arg: bad",
		},
		{
			desc:      "unknown out argument",
			name:      "foo",
			inArgMap:  map[string]struct{}{"args": {}},
			outArgMap: map[string]struct{}{"resp_body": {}, "bad": {}},
			errString: "unknown arg: bad",
		},
		{
			desc:      "missing status and resp_body",
			name:      "foo",
			inArgMap:  map[string]struct{}{"args": {}},
			outArgMap: map[string]struct{}{},
			errString: "missing status, resp_body, and template out arguments",
		},
	} {
		h, err := server.NewPGFuncHandler(tt.name, tt.inArgMap, tt.outArgMap)
		assert.EqualErrorf(t, err, tt.errString, "%d: %s", i, tt.desc)
		assert.Nilf(t, h, "%d: %s", i, tt.desc)
	}
}
