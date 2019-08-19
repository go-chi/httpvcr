package httpvcr

import (
	"net/http"
)

type Mode uint32

const (
	ModeStopped Mode = iota
	ModeRecord
	ModeReplay
)

// RequestModifierFunc is a function that can be used to manipulate HTTP requests
// before they are sent to the server.
// Useful for adding row-limits in integration tests.
type RequestModifierFunc func(request *http.Request)

type HTTPVCR struct {
	options Options

	mode            Mode
	Cassette        *cassette
	FilterMap       map[string]string
	RequestModifier RequestModifierFunc

	originalTransport http.RoundTripper
}

type Options struct {
	HTTPDefaultOverride bool
}

var DefaultOptions = Options{
	HTTPDefaultOverride: true,
}

func New(cassetteName string, opts ...Options) *HTTPVCR {
	options := DefaultOptions
	if len(opts) > 0 {
		options = opts[0]
	}

	return &HTTPVCR{
		options:   options,
		mode:      ModeStopped,
		Cassette:  &cassette{name: cassetteName},
		FilterMap: make(map[string]string),
	}
}

// Start starts a VCR session with the given cassette name.
// Records episodes if the cassette file does not exists.
// Otherwise plays back recorded episodes.
func (v *HTTPVCR) Start() {
	if v.mode != ModeStopped {
		panic("httpvcr: session already started!")
	}

	v.originalTransport = http.DefaultTransport
	if v.options.HTTPDefaultOverride {
		http.DefaultTransport = v
	}

	if v.Cassette.Exists() {
		v.mode = ModeReplay
		v.Cassette.read()
	} else {
		v.mode = ModeRecord
	}
}

// Stop stops the VCR session and writes the cassette file (when recording)
func (v *HTTPVCR) Stop() {
	if v.mode == ModeRecord {
		v.Cassette.write()
	}

	if v.options.HTTPDefaultOverride { //&& v.originalTransport != nil {
		http.DefaultTransport = v.originalTransport
	}

	v.mode = ModeStopped
}

func (v *HTTPVCR) Mode() Mode {
	return v.mode
}

// FilterData allows replacement of sensitive data with a dummy-string
func (v *HTTPVCR) FilterResponseBody(original string, replacement string) {
	v.FilterMap[original] = replacement
}

func (v *HTTPVCR) RoundTrip(request *http.Request) (*http.Response, error) {
	vcrReq := newVCRRequest(request, v.FilterMap)
	var vcrRes *vcrResponse

	if v.mode == ModeStopped {
		return v.originalTransport.RoundTrip(request)
	}

	if v.RequestModifier != nil {
		v.RequestModifier(request)
	}

	if v.mode == ModeRecord {
		response, err := v.originalTransport.RoundTrip(request)
		if err != nil {
			return nil, err
		}
		vcrRes = newVCRResponse(response)

		e := episode{Request: vcrReq, Response: vcrRes}
		v.Cassette.Episodes = append(v.Cassette.Episodes, e)

	} else {
		e := v.Cassette.matchEpisode(vcrReq)
		vcrRes = e.Response
	}

	return vcrRes.httpResponse(), nil
}
