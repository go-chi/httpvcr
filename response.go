package httpvcr

import (
	"bytes"
	"io/ioutil"
	"net/http"
)

type VCRResponse struct {
	Status        string
	StatusCode    int
	ContentLength int64
	Header        http.Header
	Body          []byte
}

func newVCRResponse(response *http.Response) *VCRResponse {
	var body []byte
	if response.Body != nil {
		body, _ = ioutil.ReadAll(response.Body)
	}

	return &VCRResponse{
		Status:        response.Status,
		StatusCode:    response.StatusCode,
		Header:        response.Header,
		ContentLength: response.ContentLength,
		Body:          body,
	}
}

func (vr *VCRResponse) httpResponse() *http.Response {
	return &http.Response{
		Status:        vr.Status,
		StatusCode:    vr.StatusCode,
		Proto:         "HTTP/1.0",
		ProtoMajor:    1,
		ProtoMinor:    0,
		Header:        vr.Header,
		ContentLength: vr.ContentLength,
		Body:          ioutil.NopCloser(bytes.NewBuffer(vr.Body)),
	}
}
