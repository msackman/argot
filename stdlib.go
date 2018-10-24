package argot

import (
	"fmt"
	"reflect"

	"github.com/kylelemons/godebug/pretty"
)

// ExpectNil is a Step that checks if the value is nil.
func ExpectNil(actual interface{}) *NamedStep {
	return NewNamedStep("ExpectNil", func() error {
		if actual == nil {
			return nil
		}
		return fmt.Errorf("Expected %#v, got %#v", nil, actual)
	})
}

// ExpectDeepEqual is a Step that checks if the actual and expected values are
// the same, as determined by reflect.DeepEqual.  The error message will
// mention both the Go representation as well as the type of both values.
func ExpectDeepEqual(actual interface{}, expected interface{}) *NamedStep {
	return NewNamedStep("ExpectDeepEqual", func() error {
		if !reflect.DeepEqual(actual, expected) {
			return fmt.Errorf("Expected %#[1]v (%[1]T), got %#[2]v (%[2]T)", actual, expected)
		}
		return nil
	})
}

// ExpectError is a Step that checks if the supplied step returns an error.
func ExpectError(step Step) *NamedStep {
	return NewNamedStep(fmt.Sprintf("ExpectError(%v)", step), func() error {
		if step.Go() == nil {
			return fmt.Errorf("Expected error from step, got %#v", nil)
		}
		return nil
	})
}

// ExpectDiffEqual is a Step that checks if the actual and expected strings are
// the same and returns a formatted error via diffmatchpatch.DiffPrettyText if
// there were any differences.
func ExpectDiffEqual(actual string, expected string) *NamedStep {
	return NewNamedStep("ExpectDiffEqual", func() error {
		if actual != expected {
			return fmt.Errorf("Expected equal strings, got diff: %s", diff(actual, expected))
		}
		return nil
	})
}

// ExpectPrettyEqual is a Step that checks if the actual and expected values
// are the same, as determined by pretty.Compare.  The error message will be a
// structured diff with each line indicating a missing or additional value with
// "-" and "+".
func ExpectPrettyEqual(actual interface{}, expected interface{}) *NamedStep {
	return NewNamedStep("ExpectPrettyEqual", func() error {
		diff := pretty.Compare(actual, expected)
		if diff != "" {
			return fmt.Errorf("Expected equal values, got diff: (-have +want)\n%s", diff)
		}
		return nil
	})
}
