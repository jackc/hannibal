package appconf_test

import (
	"testing"

	"github.com/jackc/hannibal/appconf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	yml := []byte(`
routes:
  - path: /foo/bar
    func: get_bar
    params:
      - name: foo
      - name: bar

  - path: /baz/:id
    func: get_baz
    params:
      - name: id
        type: int
`)

	config, err := appconf.New(yml)
	require.NoError(t, err)
	require.NotNil(t, config)

	require.Len(t, config.Routes, 2)
	{
		r := config.Routes[0]
		assert.Equal(t, "/foo/bar", r.Path)
		assert.Equal(t, "get_bar", r.Func)
		require.Len(t, r.Params, 2)
		{
			p := r.Params[0]
			assert.Equal(t, "foo", p.Name)
		}
		{
			p := r.Params[1]
			assert.Equal(t, "bar", p.Name)
		}
	}
	{
		r := config.Routes[1]
		assert.Equal(t, "/baz/:id", r.Path)
		assert.Equal(t, "get_baz", r.Func)
		require.Len(t, r.Params, 1)
		{
			p := r.Params[0]
			assert.Equal(t, "id", p.Name)
			assert.Equal(t, "int", p.Type)
		}
	}
}

func TestConfigLoadEmpty(t *testing.T) {
	config, err := appconf.Load("testdata/empty")
	assert.EqualError(t, err, "no yml files found in testdata/empty")
	assert.Nil(t, config)
}

func TestConfigSingleFile(t *testing.T) {
	config, err := appconf.Load("testdata/singlefile")
	require.NoError(t, err)
	require.NotNil(t, config)

	require.Len(t, config.Routes, 2)
	{
		r := config.Routes[0]
		assert.Equal(t, "/foo/bar", r.Path)
		assert.Equal(t, "get_bar", r.Func)
		require.Len(t, r.Params, 2)
		{
			p := r.Params[0]
			assert.Equal(t, "foo", p.Name)
		}
		{
			p := r.Params[1]
			assert.Equal(t, "bar", p.Name)
		}
	}
	{
		r := config.Routes[1]
		assert.Equal(t, "/baz/:id", r.Path)
		assert.Equal(t, "get_baz", r.Func)
		require.Len(t, r.Params, 1)
		{
			p := r.Params[0]
			assert.Equal(t, "id", p.Name)
			assert.Equal(t, "int", p.Type)
		}
	}
}
