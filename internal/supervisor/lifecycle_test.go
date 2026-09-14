package supervisor

import "testing"

func TestLifecycleTransitionsThroughNormalShutdown(t *testing.T) {
	lifecycle := newLifecycle()

	if got := lifecycle.State(); got != StateStarting {
		t.Fatalf("initial state = %q, want %q", got, StateStarting)
	}

	for _, next := range []State{
		StateReady,
		StateStopping,
		StateStopped,
	} {
		if err := lifecycle.Transition(next); err != nil {
			t.Fatalf("Transition(%q) error = %v", next, err)
		}

		if got := lifecycle.State(); got != next {
			t.Fatalf(
				"state after Transition(%q) = %q, want %q",
				next,
				got,
				next,
			)
		}
	}
}

func TestLifecycleRejectsInvalidTransition(t *testing.T) {
	lifecycle := newLifecycle()

	err := lifecycle.Transition(StateStopped)
	if err == nil {
		t.Fatal("Transition(StateStopped) error = nil, want an error")
	}

	if got := lifecycle.State(); got != StateStarting {
		t.Fatalf(
			"state after rejected transition = %q, want %q",
			got,
			StateStarting,
		)
	}
}
