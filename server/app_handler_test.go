package server_test

import (
	"testing"

	"github.com/gofrs/uuid"
	"github.com/jackc/hannibal/server"
	"github.com/shopspring/decimal"
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
		{
			desc: "uuid from string in standard uuid format",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeUUID,
			},
			value:  "8104307f-4ccb-469d-bfa2-352cd0c57dfa",
			result: uuid.Must(uuid.FromString("8104307f-4ccb-469d-bfa2-352cd0c57dfa")),
		},
		{
			desc: "decimal from string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeDecimal,
			},
			value:  "123",
			result: decimal.RequireFromString("123"),
		},
		{
			desc: "decimal from float64",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeDecimal,
			},
			value:  float64(123),
			result: decimal.NewFromFloat(123),
		},
		{
			desc: "boolean from boolean",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBoolean,
			},
			value:  true,
			result: true,
		},
		{
			desc: "boolean from string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBoolean,
			},
			value:  "t",
			result: true,
		},
		{
			desc: "boolean from number",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBoolean,
			},
			value:  float64(1),
			result: true,
		},
		{
			desc: "array from untyped array",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeArray,
			},
			value:  []interface{}{"foo"},
			result: []interface{}{"foo"},
		},
		{
			desc: "array from typed array",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeArray,
				ArrayElement: &server.RequestParam{
					Type: server.RequestParamTypeInt,
				},
			},
			value:  []interface{}{"42"},
			result: []interface{}{int32(42)},
		},
		{
			desc: "object unconstrained",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeObject,
			},
			value:  map[string]interface{}{"foo": "bar", "baz": float64(42)},
			result: map[string]interface{}{"foo": "bar", "baz": float64(42)},
		},
		{
			desc: "object from typed object",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeObject,
				ObjectFields: []*server.RequestParam{
					{
						Name: "foo",
						Type: server.RequestParamTypeText,
					},
					{
						Name: "baz",
						Type: server.RequestParamTypeInt,
					},
				},
			},
			value:  map[string]interface{}{"foo": "bar", "baz": "42", "ignored": "ignored"},
			result: map[string]interface{}{"foo": "bar", "baz": int32(42)},
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
		{
			desc: "decimal from non-numeric string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeDecimal,
			},
			value:  "abc",
			errStr: "not a number",
		},
		{
			desc: "boolean from non-boolean string",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeBoolean,
			},
			value:  "abc",
			errStr: "not a boolean",
		},
		{
			desc: "uuid from non-uuid",
			rp: &server.RequestParam{
				Type: server.RequestParamTypeUUID,
			},
			value:  "abcde",
			errStr: "not a uuid",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			result, err := tt.rp.Parse(tt.value)
			require.EqualError(t, err, tt.errStr)
			require.Nil(t, result)
		})
	}
}
