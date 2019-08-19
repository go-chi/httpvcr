package httpvcr

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModifyHTTPRequestBody(t *testing.T) {
	req, err := http.NewRequest("GET", "/", bytes.NewBufferString("abc"))
	require.Nil(t, err)
	require.Equal(t, int64(3), req.ContentLength)

	ModifyHTTPRequestBody(req, func(input string) string {
		require.Equal(t, input, "abc")
		return "foofoo"
	})

	require.Equal(t, int64(6), req.ContentLength)
	body, _ := ioutil.ReadAll(req.Body)
	require.Equal(t, "foofoo", string(body))
}

func TestModifyHTTPRequestBodyWithNilBody(t *testing.T) {
	req, err := http.NewRequest("GET", "/", nil)
	require.Nil(t, err)
	require.Equal(t, int64(0), req.ContentLength)

	ModifyHTTPRequestBody(req, func(input string) string {
		return "foofoo"
	})

	require.Equal(t, int64(0), req.ContentLength)
	require.Equal(t, req.Body, nil)
}
