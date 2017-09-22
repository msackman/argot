package argot

import (
	"errors"
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
