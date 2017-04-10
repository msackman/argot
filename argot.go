package argot

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/xeipuuv/gojsonschema"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

type Step interface {
	Go() error
}

type Steps []Step

// t can be nil. If t is not nil and an error occurs, then t.Fatal
// will be called. The steps returned represent the steps that were
// executed, including the step that errored, if an error occurred.
func (ss Steps) Go(t *testing.T) (results Steps, err error) {
	if t != nil {
		defer func() {
			if err != nil {
				t.Fatalf("Achieved Steps: %v; Error: %v", results, err)
			}
		}()
	}
	results = make([]Step, 0, len(ss))
	for _, step := range ss {
		results = append(results, step)
		err = step.Go()
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// Use a chan to indicate the steps to carry out rather than a
// slice. The advantage of a chan is that it allows more laziness:
// steps can even be responsible for issuing their own subsequent
// steps.
type StepsChan <-chan Step

// t can be nil. If t is not nil and an error occurs, then t.Fatal
// will be called. The steps returned represent the steps that were
// executed, including the step that errored, if an error occurred.
func (sc StepsChan) Go(t *testing.T) (results Steps, err error) {
	if t != nil {
		defer func() {
			if err != nil {
				t.Fatalf("Achieved Steps: %v; Error: %v", results, err)
			}
		}()
	}
	results = make([]Step, 0, 16)
	for step := range sc {
		results = append(results, step)
		err = step.Go()
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

type StepFunc func() error

func (sf StepFunc) Go() error {
	return sf()
}

type NamedStep struct {
	StepFunc
	name string
}

func (ns NamedStep) String() string {
	return ns.name
}

func NewNamedStep(name string, step StepFunc) *NamedStep {
	return &NamedStep{
		StepFunc: step,
		name:     name,
	}
}

type HttpCall struct {
	Client       *http.Client
	Request      *http.Request
	Response     *http.Response
	ResponseBody []byte
}

// client can be nil. If it is nil, a new http.Client is used.
func NewHttpCall(client *http.Client) *HttpCall {
	if client == nil {
		client = new(http.Client)
	}
	return &HttpCall{
		Client: client,
	}
}

func (hc *HttpCall) AssertNoRequest() error {
	if hc.Request == nil {
		return nil
	} else {
		return errors.New("Request already set")
	}
}

func (hc *HttpCall) AssertRequest() error {
	if hc.Request == nil {
		return errors.New("No Request set")
	} else {
		return nil
	}
}

func (hc *HttpCall) AssertNoRespose() error {
	if hc.Response == nil {
		return nil
	} else {
		return errors.New("Response already set")
	}
}

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

// Idempotent.
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

// Idempotent. You should ensure this is called at the end of life for
// each HttpCall. It drains bodies and generally sweeps up the mess.
func (hc *HttpCall) Reset() error {
	hc.Request = nil
	if hc.Response != nil && hc.ResponseBody == nil {
		io.Copy(ioutil.Discard, hc.Response.Body)
		hc.Response.Body.Close()
		hc.Response = nil
	}
	hc.ResponseBody = nil
	return nil
}

// This will automatically call HttpCall.Reset to ensure it's safe to
// create a new request.
func NewRequest(hc *HttpCall, method, urlStr string, body io.Reader) Step {
	return NewNamedStep("NewRequest", func() error {
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

func RequestHeader(hc *HttpCall, key, value string) Step {
	return NewNamedStep("RequestHeader", func() error {
		if err := AnyError(hc.AssertRequest(), hc.AssertNoRespose()); err != nil {
			return err
		} else {
			hc.Request.Header.Set(key, value)
			return nil
		}
	})
}

func ResponseStatusEquals(hc *HttpCall, status int) Step {
	return NewNamedStep("ResponseStatusEquals", func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if hc.Response.StatusCode != status {
			return fmt.Errorf("Status: Expected %d; found %d.", status, hc.Response.StatusCode)
		} else {
			return nil
		}
	})
}

func ResponseHeaderEquals(hc *HttpCall, key, value string) Step {
	return NewNamedStep("ResponseHeaderEquals", func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if header := hc.Response.Header.Get(key); header != value {
			return fmt.Errorf("Header '%s': Expected '%s'; found '%s'.", key, value, header)
		} else {
			return nil
		}
	})
}

func ResponseHeaderContains(hc *HttpCall, key, value string) Step {
	return NewNamedStep("ResponseHeaderContains", func() error {
		if err := hc.EnsureResponse(); err != nil {
			return err
		} else if header := hc.Response.Header.Get(key); !strings.Contains(header, value) {
			return fmt.Errorf("Header '%s': Expected '%s'; found '%s'.", key, value, header)
		} else {
			return nil
		}
	})
}

func ResponseBodyEquals(hc *HttpCall, value string) Step {
	return NewNamedStep("ResponseBodyEquals", func() error {
		if err := hc.ReceiveBody(); err != nil {
			return err
		} else if string(hc.ResponseBody) != value {
			return fmt.Errorf("Body: Expected '%s'; found '%s'.", value, string(hc.ResponseBody))
		} else {
			return nil
		}
	})
}

func ResponseBodyContains(hc *HttpCall, value string) Step {
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

func ResponseBodyJSONSchema(hc *HttpCall, schema string) Step {
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

func AnyError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
