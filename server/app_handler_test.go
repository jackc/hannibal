package server_test

import (
	"testing"

	"github.com/jackc/hannibal/server"
	"github.com/stretchr/testify/require"
)

func TestRequestParamParse(t *testing.T) {
	// Success cases
	for _, tt := range []struct {
		desc   string
		rp     *server.RequestParam
		value  interface{}
		result interface{}
	}{
		{
			desc: "text from text",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeText,
			},
			value:  "foo",
			result: "foo",
		},
		{
			desc: "text with trim space",
			rp: &server.RequestParam{
				Type:      server.RequestParamTypeText,
				TrimSpace: true,
			},
			value:  "  foo   ",
			result: "foo",
		},
		{
			desc: "text without trim space",
			rp: &server.RequestParam{
				Type:      server.RequestParamTypeText,
				TrimSpace: false,
			},
			value:  "  foo   ",
			result: "  foo   ",
		},
		{
			desc: "text from number",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeText,
			},
			value:  123,
			result: "123",
		},
		{
			desc: "int from string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeInt,
			},
			value:  "123",
			result: int32(123),
		},
		{
			desc: "int from string with trim space",
			rp: &server.RequestParam{
				Type:      server.RequestParamTypeInt,
				TrimSpace: true,
			},
			value:  "   123 ",
			result: int32(123),
		},
		{
			desc: "int from float64",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeInt,
			},
			value:  float64(123),
			result: int32(123),
		},
		{
			desc: "bigint from string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBigint,
			},
			value:  "123",
			result: int64(123),
		},
		{
			desc: "bigint from string with trim space",
			rp: &server.RequestParam{
				Type:      server.RequestParamTypeBigint,
				TrimSpace: true,
			},
			value:  "   123 ",
			result: int64(123),
		},
		{
			desc: "bigint from float64",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBigint,
			},
			value:  float64(123),
			result: int64(123),
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			result, err := tt.rp.Parse(tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.result, result)
		})
	}

	// Failure cases
	for _, tt := range []struct {
		desc   string
		rp     *server.RequestParam
		value  interface{}
		errStr string
	}{
		{
			desc: "text required",
			rp: &server.RequestParam{
				Type:     server.RequestParamTypeText,
				Required: true,
			},
			value:  nil,
			errStr: "missing",
		},
		{
			desc: "text required with empty string and nullify empty",
			rp: &server.RequestParam{
				Type:         server.RequestParamTypeText,
				Required:     true,
				NullifyEmpty: true,
			},
			value:  "",
			errStr: "missing",
		},
		{
			desc: "int from non-numeric string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeInt,
			},
			value:  "abc",
			errStr: "not a number",
		},
		{
			desc: "int from too big numeric string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeInt,
			},
			value:  "99999999999999999",
			errStr: "out of range",
		},
		{
			desc: "bigint from non-numeric string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBigint,
			},
			value:  "abc",
			errStr: "not a number",
		},
		{
			desc: "bigint from too big numeric string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBigint,
			},
			value:  "999999999999999999999999999999",
			errStr: "out of range",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			result, err := tt.rp.Parse(tt.value)
			require.EqualError(t, err, tt.errStr)
			require.Nil(t, result)
		})
	}
}
