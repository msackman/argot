package argot

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kylelemons/godebug/pretty"
)

// Step represents a step in a test.
type Step interface {
	Go() error
}

// If we can have a single Step, then we can have a slice of Steps
// representing the order of Steps in a larger unit.
type Steps []Step

var (
	defaultConfig = &pretty.Config{
		Formatter: map[reflect.Type]interface{}{
			reflect.TypeOf((*Step)(nil)).Elem(): fmt.Sprint,
		},
	}
)

func formatFatalSteps(results Steps, err error) string {
	msg := ""
	l := len(results)
	if l > 1 {
		msg = "Achieved Steps:\n" + defaultConfig.Sprint(results[:l-1]) + "\n"
	}
	if l > 0 {
		msg = msg + "Failed Step:\n" + defaultConfig.Sprint(&results[l-1]) + "\n"
	}
	return fmt.Sprintf("%vError: %v", msg, err)
}

// Test runs the steps in order and returns either all the steps, or
// all the steps that did not error, plus the step that errored. Thus
// the results are always a prefix of the Steps.  t can be nil. If t
// is not nil and an error occurs, then t.Fatal will be called. If an
// error occurs, it will be returned.
func (ss Steps) Test(t *testing.T) (results Steps, err error) {
	if t != nil {
		defer func() {
			if err != nil {
				t.Fatalf(formatFatalSteps(results, err))
			}
		}()
	}
	results, err = ss.run()
	return
}

func (ss Steps) Go() error {
	_, err := ss.run()
	return err
}

func (ss Steps) run() (Steps, error) {
	var err error
	for idx, step := range ss {
		err = step.Go()
		if err != nil {
			return ss[:idx+1], err
		}
	}
	return ss, nil
}

// StepFunc is the basic type of a Step: a function that takes no
// arguments and returns an error.
type StepFunc func() error

func (sf StepFunc) Go() error {
	return sf()
}

// NamedStep extends StepFunc by adding a name, which is mainly of use
// when formatting a Step.
type NamedStep struct {
	StepFunc
	name string
}

func (ns NamedStep) String() string {
	return ns.name
}

// NewNamedStep creates a NamedStep with the given name and Step
// function.
func NewNamedStep(name string, step StepFunc) *NamedStep {
	return &NamedStep{
		StepFunc: step,
		name:     name,
	}
}

// AnyError is a utility function that returns the first non-nil error
// in the slice, or nil if either the slice or all elements of the
// slice are nil.
func AnyError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
