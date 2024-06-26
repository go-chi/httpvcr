package httpvcr

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var testRequestCounter = 0

func testBegin(t *testing.T) {
	// delete old fixtures
	err := os.RemoveAll("fixtures")
	assert.Nil(t, err)

	// reset counter
	testRequestCounter = 0
}

func testRequest(t *testing.T, url string, postBody *string) (*http.Response, string) {
	var err error
	var response *http.Response

	if postBody != nil {
		buf := bytes.NewBufferString(*postBody)
		response, err = http.Post(url, "text/plain", buf)
	} else {
		response, err = http.Get(url)
	}
	assert.Nil(t, err)
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	assert.Nil(t, err)

	return response, string(body)
}

func testAllRequests(t *testing.T, urlBase string) {
	var body string
	var response *http.Response

	response, body = testRequest(t, urlBase, nil)
	assert.Equal(t, "0:GET:/:''", body)
	assert.Equal(t, 200, response.StatusCode)
	assert.Equal(t, "200 OK", response.Status)
	assert.Equal(t, "HTTP/1.0", response.Proto)
	assert.Equal(t, 1, response.ProtoMajor)
	assert.Equal(t, 0, response.ProtoMinor)
	assert.Equal(t, len(body), int(response.ContentLength))
	assert.Equal(t, []string{"yes"}, response.Header["Test"])

	_, body = testRequest(t, urlBase, nil)
	assert.Equal(t, "1:GET:/:''", body)

	str := "Hey Buddy"
	response, body = testRequest(t, urlBase, &str)
	assert.Equal(t, "2:POST:/:'Hey Buddy'", body)
	assert.Equal(t, len(body), int(response.ContentLength))

	multilineStr := "abc\ndef\n"
	_, body = testRequest(t, urlBase, &multilineStr)
	assert.Equal(t, "3:POST:/:'abc\ndef\n'", body)

	_, body = testRequest(t, urlBase+"/modme", &str)
	assert.Equal(t, "4:POST:/modme:'moddedString'", body)

	str = "secret-key"
	_, body = testRequest(t, urlBase, &str)
	assert.Equal(t, "5:POST:/:'secret-key'", body)

}

func testServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header["Test"] = []string{"yes"}

		body, _ := ioutil.ReadAll(r.Body)
		fmt.Fprintf(w, "%d:%s:%s:'%s'", testRequestCounter, r.Method, r.URL.Path, body)
		testRequestCounter++
	}))
}

func requestMod(mode Mode, request *http.Request) {
	if request.URL.Path == "/modme" {
		ModifyHTTPRequestBody(request, func(body string) string {
			return strings.Replace(string(body), "Hey Buddy", "moddedString", 1)
		})
	}
}

func TestVCR(t *testing.T) {
	testBegin(t)

	ts := testServer()
	defer ts.Close()

	vcr := New("test_cassette")
	vcr.FilterResponseBody("secret-key", "dummy-key")
	vcr.BeforeRequest = requestMod

	vcr.Start(context.Background())
	assert.Equal(t, ModeRecord, vcr.Mode())
	testAllRequests(t, ts.URL)
	vcr.Stop()

	// this only works because the key is the only body content.
	// otherwise the base64 alignment would be off.
	data, _ := ioutil.ReadFile("fixtures/vcr/test_cassette.json")
	assert.Contains(t, string(data), base64.StdEncoding.EncodeToString([]byte("dummy-key")))
	assert.NotContains(t, string(data), base64.StdEncoding.EncodeToString([]byte("secret-key")))

	vcr.Start(context.Background())
	assert.Equal(t, ModeReplay, vcr.Mode())
	testAllRequests(t, ts.URL)
	vcr.Stop()
}

func TestNoSession(t *testing.T) {
	testBegin(t)

	ts := testServer()
	defer ts.Close()

	_, body := testRequest(t, ts.URL, nil)
	assert.Equal(t, "0:GET:/:''", body)
}

func TestNoEpisodesLeft(t *testing.T) {
	testBegin(t)

	defer func() {
		assert.Equal(t, "httpvcr: no more episodes!", recover())
	}()

	vcr := New("test_cassette")
	vcr.Start(context.Background())
	vcr.Stop()

	vcr.Start(context.Background())
	defer vcr.Stop()
	testRequest(t, "http://1.2.3.4", nil)
}

func TestEpisodesDoNotMatch(t *testing.T) {
	testBegin(t)

	ts := testServer()
	defer ts.Close()

	vcr := New("test_cassette")
	assert.Equal(t, ModeStopped, vcr.Mode())
	vcr.Start(context.Background())
	assert.Equal(t, ModeRecord, vcr.Mode())
	testRequest(t, ts.URL, nil)
	vcr.Stop()

	// Method mismatch
	func() {
		vcr.Start(context.Background())
		defer vcr.Stop()

		defer func() {
			assert.Equal(t, fmt.Sprintf("httpvcr: problem with episode for POST %s\n  episode Method does not match:\n  expected: GET\n  but got: POST", ts.URL), recover())
		}()

		body := ""
		testRequest(t, ts.URL, &body)
	}()

	// URL mismatch
	func() {
		otherURL := ts.URL + "/abc"
		defer func() {
			assert.Equal(t, fmt.Sprintf("httpvcr: problem with episode for GET %s\n  episode URL does not match:\n  expected: %v\n  but got: %v", otherURL, ts.URL, otherURL), recover())
		}()

		vcr.Start(context.Background())
		defer vcr.Stop()
		testRequest(t, otherURL, nil)
	}()

	func() {
		defer func() {
			assert.Equal(t, fmt.Sprintf("httpvcr: problem with episode for POST %s\n  episode Body does not match:\n  expected: foo\n  but got: bar", ts.URL), recover())
		}()

		body := "foo"

		vcr = New("test_cassette2")

		vcr.Start(context.Background())
		testRequest(t, ts.URL, &body)
		vcr.Stop()

		vcr.Start(context.Background())
		defer vcr.Stop()
		body = "bar"
		testRequest(t, ts.URL, &body)
	}()
}

func TestOriginalRoundTripErrors(t *testing.T) {
	testBegin(t)

	vcr := New("test_cassette")
	vcr.Start(context.Background())
	defer vcr.Stop()

	_, err := http.Get("xhttp://foo")
	assert.EqualError(t, err, "Get \"xhttp://foo\": unsupported protocol scheme \"xhttp\"")
}

func TestFileWriteError(t *testing.T) {
	testBegin(t)

	defer func() {
		assert.Equal(t, recover(), "httpvcr: cannot write cassette file!")
	}()

	vcr := New("test")
	vcr.Start(context.Background())
	defer vcr.Stop()

	err := os.MkdirAll("fixtures/vcr/test.json", 0755)
	assert.Nil(t, err)
}

func TestFileParseError(t *testing.T) {
	testBegin(t)

	defer func() {
		assert.Equal(t, recover(), "httpvcr: cannot parse json!")
	}()

	os.MkdirAll("fixtures/vcr", 0755)
	err := ioutil.WriteFile("fixtures/vcr/test.json", []byte("{[}"), 0644)
	assert.Nil(t, err)

	vcr := New("test")
	vcr.Start(context.Background())
	vcr.Stop()
}

func TestStartTwice(t *testing.T) {
	testBegin(t)

	defer func() {
		assert.Equal(t, recover(), "httpvcr: session already started!")
	}()

	vcr := New("test")
	vcr.Start(context.Background())
	vcr.Start(context.Background())

	vcr.Stop()
}
