package argot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"regexp"
	"strings"

	"github.com/kylelemons/godebug/pretty"
	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/xeipuuv/gojsonschema"
)

// HttpCall captures all the state relating to a single HTTP call. It
// may be used multiple times. An HttpCall can only be used by a
// single go-routine at a time.
type HttpCall struct {
	// The client used to perform the request.
	Client *http.Client
	// The request to be made.
	Request *http.Request
	// The response.
	Response *http.Response
	// The body which once received can be repeatedly reused.
	ResponseBody []byte
}

// NewHttpCall creates a new HttpCall. If client is nil, a new
// http.Client is used.
func NewHttpCall(client *http.Client) *HttpCall {
	if client == nil {
		client = new(http.Client)
	}
	return &HttpCall{
		Client: client,
	}
}

// AssertNoRequest returns nil iff hc.Request is nil.
func (hc *HttpCall) AssertNoRequest() error {
	if hc.Request == nil {
		return nil
	} else {
		return errors.New("Request already set")
	}
}

// AssertRequest returns nil iff hc.Request is non-nil.
func (hc *HttpCall) AssertRequest() error {
	if hc.Request == nil {
		return errors.New("No Request set")
	} else {
		return nil
	}
}

// AssertNoResponse returns nil iff hc.Response is nil.
func (hc *HttpCall) AssertNoResponse() error {
	if hc.Response == nil {
		return nil
	} else {
		return errors.New("Response already set")
	}
}

// EnsureResponse is idempotent. If there is already a response then
// it will return nil. Otherwise if there is no Request then it will
// return non-nil. Otherwise it will use hc.Client.Do to perform the
// request, set hc.Response, and return any error that occurs.
//
// Always use this in any step where you want to inspect the
// hc.Response.
func (hc *HttpCall) EnsureResponse() error {
	if hc.Response != nil {
		return nil
	} else if hc.Request == nil {
		return errors.New("Cannot ensure response: no request.")
	} else if response, err := hc.Client.Do(hc.Request); err != nil {
		return fmt.Errorf("Error when making call of %v: %v", hc.Request, err)
	} else {
		hc.Response = response
		return nil
	}
}

// ReceiveBody is idempotent. It will ensure there is a response using
// hc.EnsureResponse. If there is already a non-nil hc.ResponseBody
// then it will return nil. Otherwise it will receive the
// Response.Body, store it in hc.ResponseBody, and return any error
// that occurs.
//
// Always use this in any step where you want to inspect the
// hc.ResponseBody.
func (hc *HttpCall) ReceiveBody() error {
	if err := hc.EnsureResponse(); err != nil {
		return err
	} else if hc.ResponseBody != nil {
		return nil
	} else {
		defer hc.Response.Body.Close()
		bites := new(bytes.Buffer)
		if _, err = io.Copy(bites, hc.Response.Body); err != nil {
			return err
		} else {
			hc.ResponseBody = bites.Bytes()
			return nil
		}
	}
}

// Reset is idempotent. You should ensure this is called at the end of
// life for each HttpCall. It drains Response bodies if necessary, and
// cleans up resources.
func (hc *HttpCall) Reset() error {
	hc.Request = nil
	if hc.Response != nil && hc.ResponseBody == nil {
		io.Copy(ioutil.Discard, hc.Response.Body)
		hc.Response.Body.Close()
	}
	hc.Response = nil
	hc.ResponseBody = nil
	return nil
}

// NewRequest is a Step that when executed will create a new request
// using the given parameters. The step will automatically call
// hc.Reset to tidy up any previous use of hc, and thus prepare hc for
// the new request.
func (hc *HttpCall) NewRequest(method, urlStr string, body io.Reader) Step {
	return NewNamedStep(fmt.Sprintf("NewRequest(%s: %s)", method, urlStr), func() error {
		if err := hc.Reset(); err != nil {
			return err
		} else if req, err := http.NewRequest(method, urlStr, body); err != nil {
			return err
		} else {
			hc.Request = req
			return nil
		}
	})
}

// RequestHeader is a Step that when executed will set the given key
// and value as a header on the HTTP Request. This can only be done
// after hc.Request has been created (with NewRequest), and before
// hc.Response has been created.
func (hc *HttpCall) RequestHeader(key, value string) Step {
	return NewNamedStep(fmt.Sprintf("RequestHeader(%s: %s)", key, value), func() error {
		if err := AnyError(hc.AssertRequest(), hc.AssertNoResponse()); err != nil {
			return err
		} else {
			hc.Request.Header.Set(key, value)
			return nil
		}
	})
}

// Call is a Step that when executed performs the HTTP Request
// Call. This is not normally necessary: all steps that require a
// Response will perform the HTTP Request when necessary. However, in
// some tests, you may not care about inspecting the HTTP Response but
// nevertheless wish the HTTP Request to be made.
func (hc *HttpCall) Call() Step {
	return NewNamedStep("Call", hc.EnsureResponse)
}

// ResponseStatusEquals is a Step that when executed ensures there is
// a non-nil hc.Response and errors unless the hc.Response.StatusCode
// equals the status parameter.
func (hc *HttpCall) ResponseStatusEquals(status int) Step {
	return NewNamedStep(fmt.Sprintf("ResponseStatusEquals(%d)", status), func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if hc.Response.StatusCode != status {
			return fmt.Errorf("Status: Expected %d; found %d.", status, hc.Response.StatusCode)
		} else {
			return nil
		}
	})
}

// ResponseHeaderExists is a Step that when executed ensures there is
// a non-nil hc.Response and errors unless hc.Response.Header[key]
// exists. It says nothing about the value of the header.
func (hc *HttpCall) ResponseHeaderExists(key string) Step {
	return NewNamedStep(fmt.Sprintf("ResponseHeaderExists(%s)", key), func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if _, found := hc.Response.Header[key]; !found {
			return fmt.Errorf("Header '%s' not found.", key)
		} else {
			return nil
		}
	})
}

// ResponseHeaderNotExists is a Step that when executed ensures there
// is a non-nil hc.Response and errors unless hc.Response.Header[key]
// does not exist.
func (hc *HttpCall) ResponseHeaderNotExists(key string) Step {
	return NewNamedStep(fmt.Sprintf("ResponseHeaderNotExists(%s)", key), func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if _, found := hc.Response.Header[key]; found {
			return fmt.Errorf("Header '%s' found.", key)
		} else {
			return nil
		}
	})
}

// Diff two strings, output as coloured string, the expected parts will
// be removed/red if they're missing, the found/inserted parts will be
// green if present, if the parts are the same, no colour is applied.
func diff(expected string, got string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(expected, got, false)
	return dmp.DiffPrettyText(diffs)
}

// ResponseHeaderEquals is a Step that when executed ensures there is
// a non-nil hc.Response and errors unless the
// hc.Response.Header.Get(key) equals the value parameter. Note this
// is an exact match.
func (hc *HttpCall) ResponseHeaderEquals(key, value string) Step {
	return NewNamedStep(fmt.Sprintf("ResponseHeaderEquals(%s: %s)", key, value), func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if header := hc.Response.Header.Get(key); header != value {
			return fmt.Errorf("Header: '%s': Diff: '%s'.", key, diff(value, header))
		} else {
			return nil
		}
	})
}

// ResponseHeaderContains is a Step that when executed ensures there
// is a non-nil hc.Response and errors unless the
// hc.Response.Header.Get(key) contains the value parameter using
// strings.Contains.
func (hc *HttpCall) ResponseHeaderContains(key, value string) Step {
	return NewNamedStep(fmt.Sprintf("ResponseHeaderContains(%s: %s)", key, value), func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if header := hc.Response.Header.Get(key); !strings.Contains(header, value) {
			return fmt.Errorf("Header '%s': Expected '%s'; found '%s'.", key, value, header)
		} else {
			return nil
		}
	})
}

// ResponseBodyEquals is a Step that when executed ensures there is a
// non-nil hc.ResponseBody and errors unless the hc.ResponseBody
// equals the value parameter. Note this is an exact match.
func (hc *HttpCall) ResponseBodyEquals(value string) Step {
	return NewNamedStep("ResponseBodyEquals", func() error {
		if err := hc.ReceiveBody(); err != nil {
			return err
		} else if bodyStr := string(hc.ResponseBody); bodyStr != value {
			return fmt.Errorf("Body: Diff: '%s'.", diff(value, bodyStr))
		} else {
			return nil
		}
	})
}

// ResponseBodyContains is a Step that when executed ensures there is
// a non-nil hc.ResponseBody and errors unless the hc.ResponseBody
// contains the value parameter using strings.Contains.
func (hc *HttpCall) ResponseBodyContains(value string) Step {
	return NewNamedStep("ResponseBodyContains", func() error {
		if err := hc.ReceiveBody(); err != nil {
			return err
		} else if !strings.Contains(string(hc.ResponseBody), value) {
			return fmt.Errorf("Body: Expected '%s'; found '%s'.", value, string(hc.ResponseBody))
		} else {
			return nil
		}
	})
}

// ResponseBodyMatches is a Step that when executed ensures there is
// a non-nil hc.ResponseBody and errors unless the hc.ResponseBody
// matches the regular expression parameter.
func (hc *HttpCall) ResponseBodyMatches(pattern *regexp.Regexp) Step {
	return NewNamedStep(fmt.Sprintf("ResponseBodyMatches(%v)", pattern), func() error {
		if err := hc.ReceiveBody(); err != nil {
			return err
		} else if !pattern.MatchString(string(hc.ResponseBody)) {
			return fmt.Errorf("Body: Expected to match the pattern '%v'; found '%s'.", pattern, string(hc.ResponseBody))
		} else {
			return nil
		}
	})
}

// ResponseBodyJSONSchema is a Step that when executed ensures there
// is a non-nil hc.ResponseBody and errors unless the hc.ResponseBody
// can be validated against the schema parameter using gojsonschema.
func (hc *HttpCall) ResponseBodyJSONSchema(schema string) Step {
	return NewNamedStep("ResponseBodyJSONSchema", func() error {
		if err := hc.ReceiveBody(); err != nil {
			return err
		} else {
			schemaLoader := gojsonschema.NewStringLoader(schema)
			bodyLoader := gojsonschema.NewStringLoader(string(hc.ResponseBody))
			if result, err := gojsonschema.Validate(schemaLoader, bodyLoader); err != nil {
				return err
			} else if !result.Valid() {
				msg := "Validation failure:\n"
				for _, err := range result.Errors() {
					msg += fmt.Sprintf("\t%v\n", err)
				}
				return errors.New(msg[:len(msg)-1])
			} else {
				return nil
			}
		}
	})
}

// ResponseBodyJSONMatchesStruct is a Step that when executed ensures
// there is a non-nil hc.ResponseBody, parses it as JSON (via
// encoding/json) based on the type of the expected structure and errors
// unless it is equal to the expected value, as validated by the pretty
// package.  The error will contain a structured diff output with a
// plus/"+" marking the values that were expected and a minus/"-"
// marking the values that were actually present.
func (hc *HttpCall) ResponseBodyJSONMatchesStruct(expected interface{}) Step {
	return NewNamedStep("ResponseBodyJSONMatchesStruct", func() error {
		parseAs := reflect.New(reflect.TypeOf(expected)).Interface()
		if err := hc.ReceiveBody(); err != nil {
			return err
		} else if err := json.Unmarshal(hc.ResponseBody, parseAs); err != nil {
			return err
		} else if diff := pretty.Compare(parseAs, expected); diff != "" {
			return fmt.Errorf("Did not match expected value: (-got +want)\n%s", diff)
		} else {
			return nil
		}
	})
}
