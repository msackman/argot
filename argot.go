package argot

import (
	"bytes"
	"errors"
	"fmt"
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

type StepFunc func() error

func (sf StepFunc) Go() error {
	return sf()
}

type HttpCall struct {
	Client       *http.Client
	Request      *http.Request
	Response     *http.Response
	ResponseBody []byte
}

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

func NewRequest(hc *HttpCall, method, urlStr string, body io.Reader) Step {
	return StepFunc(func() error {
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
	return StepFunc(func() error {
		if err := AnyError(hc.AssertRequest(), hc.AssertNoRespose()); err != nil {
			return err
		} else {
			hc.Request.Header.Set(key, value)
			return nil
		}
	})
}

func ResponseStatusEquals(hc *HttpCall, status int) Step {
	return StepFunc(func() error {
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
	return StepFunc(func() error {
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
	return StepFunc(func() error {
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
	return StepFunc(func() error {
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
	return StepFunc(func() error {
		if err := hc.ReceiveBody(); err != nil {
			return err
		} else if !strings.Contains(string(hc.ResponseBody), value) {
			return fmt.Errorf("Body: Expected '%s'; found '%s'.", value, string(hc.ResponseBody))
		} else {
			return nil
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
