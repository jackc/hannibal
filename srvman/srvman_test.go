package srvman_test

import (
	"context"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/hannibal/srvman"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestLogWriter struct {
	T *testing.T
}

func (w *TestLogWriter) Write(p []byte) (int, error) {
	w.T.Log(string(p))
	return len(p), nil
}

func TestGroupSimpleLifeCycle(t *testing.T) {
	ctx := context.Background()

	makeGroup := func() *srvman.Group {
		tlr := &TestLogWriter{T: t}
		output := zerolog.ConsoleWriter{Out: tlr}
		logger := zerolog.New(output)

		return &srvman.Group{
			ServiceConfigs: []*srvman.ServiceConfig{
				{
					Name:        "http_hello",
					Cmd:         "tmp/test/bin/http_server",
					Args:        []string{"[[bluegreen.port]]"},
					HTTPAddress: "http://127.0.0.1:[[bluegreen.port]]",
					HealthCheck: &srvman.HealthCheck{
						TCPConnect: "127.0.0.1:[[bluegreen.port]]",
					},
					MaxStartupDuration: 5 * time.Second,
					Blue:               map[string]interface{}{"port": 4200},
					Green:              map[string]interface{}{"port": 4201},
					Logger:             &logger,
				},
			},
		}
	}

	blueGroup := makeGroup()

	err := blueGroup.Start(ctx, srvman.ColorBlue)
	require.NoError(t, err)
	blueStopped := false
	defer func() {
		if !blueStopped {
			blueGroup.Stop(ctx)
		}
	}()

	resp, err := http.Get(blueGroup.GetService("http_hello").HTTPAddress)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", string(body))

	greenGroup := makeGroup()

	err = greenGroup.Start(ctx, srvman.ColorGreen)
	require.NoError(t, err)
	greenStopped := false
	defer func() {
		if !greenStopped {
			greenGroup.Stop(ctx)
		}
	}()

	err = blueGroup.Stop(ctx)
	blueStopped = true
	require.NoError(t, err)

	resp, err = http.Get(greenGroup.GetService("http_hello").HTTPAddress)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", string(body))

	err = greenGroup.Stop(ctx)
	greenStopped = true
	require.NoError(t, err)
}
