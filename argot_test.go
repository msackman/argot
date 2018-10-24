package argot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestStepsSatisftyStep(t *testing.T) {
	returnErrorStep := NewNamedStep("error", func() error {
		return errors.New("test")
	})
	steps := Steps{returnErrorStep}
	var step Step = steps

	err := step.Go()
	if err == nil || err.Error() != "test" {
		t.Errorf("Unexpected error result from Steps.Go(): %s", err)
	}
}

func TestThatNilWorks(t *testing.T) {
	Steps([]Step{
		ExpectNil(nil),
		ExpectError(ExpectNil(42)),
	}).Test(t)
}

func TestThatDeepEqualWorks(t *testing.T) {
	Steps([]Step{
		ExpectDeepEqual("a", "a"),
		ExpectError(ExpectDeepEqual("a", "b")),
	}).Test(t)
}

func TestThatDiffWorks(t *testing.T) {
	Steps([]Step{
		ExpectDiffEqual("a", "a"),
		ExpectError(ExpectDeepEqual("a", "b")),
		ExpectDiffEqual(ExpectDeepEqual("a", "b").Go().Error(), `Expected "a" (string), got "b" (string)`),
	}).Test(t)
}

func TestThatPrettyWorks(t *testing.T) {
	Steps([]Step{
		ExpectPrettyEqual("a", "a"),
		ExpectError(ExpectPrettyEqual("a", "b")),
		ExpectPrettyEqual(ExpectPrettyEqual("a", "b").Go().Error(), `Expected equal values, got diff: (-have +want)
-"a"
+"b"`),
	}).Test(t)
}

func TestThatDeferredWorks(t *testing.T) {
	var a string

	Steps([]Step{
		StepFunc(func() error {
			a = "foo"
			return nil
		}),
		NewStepProducer(func() Step {
			return ExpectDiffEqual(a, "foo")
		}),
	}).Test(t)
}

func TestThatAssertRequestWorks(t *testing.T) {
	hc := NewHttpCall(nil)

	Steps([]Step{
		ExpectNil(hc.AssertNoRequest()),
		ExpectError(ExpectNil(hc.AssertRequest())),
	}).Test(t)
}

func TestThatRequestWorks(t *testing.T) {
	hc := NewHttpCall(nil)

	Steps([]Step{
		hc.NewRequest("GET", "http://localhost:8000/", nil),
		NewStepProducer(func() Step {
			return ExpectNil(hc.AssertRequest())
		}),
	}).Test(t)
}

type Sample struct {
	Foo int
}

func TestThatCallWorks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("AsDf", "")
		w.Header().Set("contains", "something")
		sampleBytes, err := json.Marshal(Sample{
			Foo: 42,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusForbidden)
			w.Write(sampleBytes)
		}
	}))
	defer ts.Close()

	hc := NewHttpCall(nil)

	Steps([]Step{
		hc.NewRequest("GET", ts.URL, nil),
		hc.Call(),
		NewStepProducer(func() Step {
			return ExpectError(ExpectNil(hc.ResponseBody))
		}),
		hc.ResponseStatusEquals(http.StatusForbidden),
		hc.ResponseHeaderExists("asDF"),
		hc.ResponseHeaderNotExists("FoO"),
		hc.ResponseHeaderContains("contains", "eth"),
		hc.ResponseBodyContains("42"),
		hc.ResponseBodyEquals(`{"Foo":42}`),
		hc.ResponseBodyMatches(regexp.MustCompile(`4.+`)),
		hc.ResponseBodyJSONSchema(`
{
  "type": "object",
  "properties": {
    "Foo": {
      "type": "integer"
    }
  }
}`),
		hc.ResponseBodyJSONMatchesStruct(Sample{
			Foo: 42,
		}),
	}).Test(t)
}
