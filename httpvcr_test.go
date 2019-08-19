package httpvcr

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var testRequestCounter = 0

func testBegin(t *testing.T) {
	// delete old fixtures
	err := os.RemoveAll("fixtures/vcr")
	require.Nil(t, err)

	// ensure no test case left us in an active state
	currentCassette = nil

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
	require.Nil(t, err)
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	require.Nil(t, err)

	return response, string(body)
}

func testAllRequests(t *testing.T, urlBase string) {
	var body string
	var response *http.Response

	response, body = testRequest(t, urlBase, nil)
	require.Equal(t, "0:GET:/:''", body)
	require.Equal(t, 200, response.StatusCode)
	require.Equal(t, "200 OK", response.Status)
	require.Equal(t, "HTTP/1.0", response.Proto)
	require.Equal(t, 1, response.ProtoMajor)
	require.Equal(t, 0, response.ProtoMinor)
	require.Equal(t, len(body), int(response.ContentLength))
	require.Equal(t, []string{"yes"}, response.Header["Test"])

	_, body = testRequest(t, urlBase, nil)
	require.Equal(t, "1:GET:/:''", body)

	str := "Hey Buddy"
	response, body = testRequest(t, urlBase, &str)
	require.Equal(t, "2:POST:/:'Hey Buddy'", body)
	require.Equal(t, len(body), int(response.ContentLength))

	multilineStr := "abc\ndef\n"
	_, body = testRequest(t, urlBase, &multilineStr)
	require.Equal(t, "3:POST:/:'abc\ndef\n'", body)

	_, body = testRequest(t, urlBase+"/modme", &str)
	require.Equal(t, "4:POST:/modme:'moddedString'", body)

	str = "secret-key"
	_, body = testRequest(t, urlBase, &str)
	require.Equal(t, "5:POST:/:'secret-key'", body)

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

func requestMod(request *http.Request) {
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

	Start("test_cassette", requestMod)
	FilterData("secret-key", "dummy-key")
	testAllRequests(t, ts.URL)
	Stop()

	// this only works because the key is the only body content.
	// otherwise the base64 alignment would be off.
	data, _ := ioutil.ReadFile("fixtures/vcr/test_cassette.json")
	require.Contains(t, string(data), base64.StdEncoding.EncodeToString([]byte("dummy-key")))
	require.NotContains(t, string(data), base64.StdEncoding.EncodeToString([]byte("secret-key")))

	Start("test_cassette", requestMod)
	FilterData("secret-key", "dummy-key")
	testAllRequests(t, ts.URL)
	Stop()
}

func TestNoSession(t *testing.T) {
	testBegin(t)

	ts := testServer()
	defer ts.Close()

	_, body := testRequest(t, ts.URL, nil)
	require.Equal(t, "0:GET:/:''", body)
}

func TestNoEpisodesLeft(t *testing.T) {
	testBegin(t)

	defer func() {
		require.Equal(t, recover(), "VCR: No more episodes!")
	}()

	Start("test_cassette", nil)
	Stop()

	Start("test_cassette", nil)
	testRequest(t, "http://1.2.3.4", nil)
}

func TestEpisodesDoNotMatch(t *testing.T) {
	testBegin(t)

	ts := testServer()
	defer ts.Close()

	Start("test_cassette", nil)
	testRequest(t, ts.URL, nil)
	Stop()

	// Method mismatch
	func() {
		defer func() {
			require.Equal(t, fmt.Sprintf("VCR: Problem with Episode for POST %s\n  Episode Method does not match:\n  expected: GET\n  but got: POST", ts.URL), recover())
		}()

		Start("test_cassette", nil)
		defer Stop()
		body := ""
		testRequest(t, ts.URL, &body)
	}()

	// URL mismatch
	func() {
		otherURL := ts.URL + "/abc"
		defer func() {
			require.Equal(t, fmt.Sprintf("VCR: Problem with Episode for GET %s\n  Episode URL does not match:\n  expected: %v\n  but got: %v", otherURL, ts.URL, otherURL), recover())
		}()

		Start("test_cassette", nil)
		defer Stop()
		testRequest(t, otherURL, nil)
	}()

	func() {
		defer func() {
			require.Equal(t, fmt.Sprintf("VCR: Problem with Episode for POST %s\n  Episode Body does not match:\n  expected: foo\n  but got: bar", ts.URL), recover())
		}()

		body := "foo"

		Start("test_cassette2", nil)
		testRequest(t, ts.URL, &body)
		Stop()

		Start("test_cassette2", nil)
		defer Stop()
		body = "bar"
		testRequest(t, ts.URL, &body)
	}()
}

func TestOriginalRoundTripErrors(t *testing.T) {
	testBegin(t)

	Start("test_cassette", nil)
	_, err := http.Get("xhttp://foo")
	require.EqualError(t, err, "Get xhttp://foo: unsupported protocol scheme \"xhttp\"")
}

func TestFileWriteError(t *testing.T) {
	testBegin(t)

	defer func() {
		require.Equal(t, recover(), "VCR: Cannot write cassette file!")
	}()

	Start("test", nil)

	err := os.MkdirAll("fixtures/vcr/test.json", 0755)
	require.Nil(t, err)

	Stop()
}

func TestFileParseError(t *testing.T) {
	testBegin(t)

	defer func() {
		require.Equal(t, recover(), "VCR: Cannot parse JSON!")
	}()

	os.MkdirAll("fixtures/vcr", 0755)
	err := ioutil.WriteFile("fixtures/vcr/test.json", []byte("{[}"), 0644)
	require.Nil(t, err)

	Start("test", nil)
}

func TestStartTwice(t *testing.T) {
	testBegin(t)

	defer func() {
		require.Equal(t, recover(), "VCR: Session already started!")
	}()

	Start("test", nil)
	Start("test", nil)
}
