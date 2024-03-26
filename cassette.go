package httpvcr

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"
)

type EpisodeMatcher interface {
	Init(episodes []Episode)
	MatchEpisode(request *VCRRequest) (*Episode, error)
}

type DefaultEpisodeMatcher struct {
	episodes []Episode
}

func (m *DefaultEpisodeMatcher) Init(episodes []Episode) {
	m.episodes = episodes
}

func (m *DefaultEpisodeMatcher) MatchEpisode(request *VCRRequest) (*Episode, error) {
	if len(m.episodes) == 0 {
		panic("httpvcr: no more episodes!")
	}

	e := m.episodes[0]
	expected := e.Request

	if expected.Method != request.Method {
		panicEpisodeMismatch(request, "Method", expected.Method, request.Method)
	}

	if expected.URL != request.URL {
		panicEpisodeMismatch(request, "URL", expected.URL, request.URL)
	}

	if !reflect.DeepEqual(expected.Body, request.Body) {
		panicEpisodeMismatch(request, "Body", string(expected.Body[:]), string(request.Body[:]))
	}

	m.episodes = m.episodes[1:]
	return &e, nil
}

type cassette struct {
	name           string
	Episodes       []Episode
	Gzip           bool
	episodeMatcher EpisodeMatcher
}

type Episode struct {
	Request  *VCRRequest
	Response *VCRResponse
}

func (c *cassette) Name() string {
	return c.name
}

func (c *cassette) Filename() string {
	if c.Gzip {
		return "fixtures/vcr/" + c.name + ".json.gz"
	} else {
		return "fixtures/vcr/" + c.name + ".json"
	}
}

func (c *cassette) Exists() bool {
	_, err := os.Stat(c.Filename())
	return err == nil
}

func (c *cassette) read() {
	var fileData, jsonData []byte

	fileData, _ = ioutil.ReadFile(c.Filename())

	if c.Gzip {
		var data bytes.Buffer
		err := gunzipWrite(&data, fileData)
		if err != nil {
			panic("httpvcr: gzip read failed")
		}
		jsonData = data.Bytes()
	} else {
		jsonData = fileData
	}

	err := json.Unmarshal(jsonData, c)
	if err != nil {
		panic("httpvcr: cannot parse json!")
	}
	c.episodeMatcher.Init(c.Episodes)
}

func (c *cassette) write() {
	jsonData, _ := json.Marshal(c)

	var jsonOut bytes.Buffer
	json.Indent(&jsonOut, jsonData, "", "  ")

	os.MkdirAll("fixtures/vcr", 0755)

	var fileOut bytes.Buffer

	if c.Gzip {
		err := gzipWrite(&fileOut, jsonOut.Bytes())
		if err != nil {
			panic("httpvcr: gzip write failed")
		}
	} else {
		fileOut = jsonOut
	}

	err := ioutil.WriteFile(c.Filename(), fileOut.Bytes(), 0644)
	if err != nil {
		panic("httpvcr: cannot write cassette file!")
	}
}

func (c *cassette) matchEpisode(request *VCRRequest) (*Episode, error) {
	return c.episodeMatcher.MatchEpisode(request)
}

func panicEpisodeMismatch(request *VCRRequest, field string, expected string, actual string) {
	panic(fmt.Sprintf(
		"httpvcr: problem with episode for %s %s\n  episode %s does not match:\n  expected: %s\n  but got: %s",
		request.Method,
		request.URL,
		field,
		expected,
		actual,
	))
}

// Write gzipped data to a Writer
func gzipWrite(w io.Writer, data []byte) error {
	// Write gzipped data to the client
	gw, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	defer gw.Close()
	gw.Write(data)
	return err
}

// Write gunzipped data to a Writer
func gunzipWrite(w io.Writer, data []byte) error {
	// Write gzipped data to the client
	gr, err := gzip.NewReader(bytes.NewBuffer(data))
	defer gr.Close()
	data, err = ioutil.ReadAll(gr)
	if err != nil {
		return err
	}
	w.Write(data)
	return nil
}
