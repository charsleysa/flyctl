package machine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/statuslogger"
	"github.com/superfly/flyctl/internal/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func WaitForStartOrStop(ctx context.Context, machine *fly.Machine, action string, timeout time.Duration) error {
	flapsClient := flapsutil.ClientFromContext(ctx)

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var waitOnAction string
	switch action {
	case "start":
		waitOnAction = "started"
	case "stop":
		waitOnAction = "stopped"
	default:
		return invalidAction
	}

	b := &backoff.Backoff{
		Min:    500 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 2,
		Jitter: false,
	}
	for {
		err := flapsClient.Wait(waitCtx, machine, waitOnAction, 60*time.Second)
		if err == nil {
			return nil
		}

		switch {
		case errors.Is(waitCtx.Err(), context.Canceled):
			return err
		case errors.Is(waitCtx.Err(), context.DeadlineExceeded):
			return WaitTimeoutErr{
				machineID:    machine.ID,
				timeout:      timeout,
				desiredState: waitOnAction,
			}
		default:
			var flapsErr *flaps.FlapsError
			if strings.Contains(err.Error(), "machine failed to reach desired state") && machine.Config.Restart != nil && machine.Config.Restart.Policy == fly.MachineRestartPolicyNo {
				return fmt.Errorf("machine failed to reach desired start state, and restart policy was set to %s restart", machine.Config.Restart.Policy)
			}
			if errors.As(err, &flapsErr) && flapsErr.ResponseStatusCode == http.StatusBadRequest {
				return fmt.Errorf("failed waiting for machine: %w", err)
			}
			time.Sleep(b.Duration())
		}
	}
}

// returns when the machine is in one of the possible states, or after passing the timeout threshold
func WaitForAnyMachineState(ctx context.Context, mach *fly.Machine, possibleStates []string, timeout time.Duration, sl statuslogger.StatusLine) (string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "wait_for_machine_state", trace.WithAttributes(
		attribute.StringSlice("possible_states", possibleStates),
	))
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	flapsClient := flapsutil.ClientFromContext(ctx)

	var mutex sync.Mutex

	var waitErr error
	numCompleted := 0
	var successfulState string

	for _, state := range possibleStates {
		state := state
		go func() {
			err := flapsClient.Wait(ctx, mach, state, timeout)
			mutex.Lock()
			defer func() {
				numCompleted += 1
				mutex.Unlock()
			}()

			if successfulState != "" {
				return
			}

			if sl != nil {
				sl.LogStatus(statuslogger.StatusRunning, fmt.Sprintf("Machine %s reached %s state", mach.ID, state))
			}

			if err != nil {
				waitErr = err
			} else {
				successfulState = state
			}
		}()
	}

	// TODO(billy): i'm sure we can use channels here
	for {
		mutex.Lock()
		if successfulState != "" || numCompleted == len(possibleStates) {
			defer mutex.Unlock()
			if successfulState != "" {
				span.SetAttributes(attribute.String("state", successfulState))
			}

			if waitErr != nil {
				span.RecordError(waitErr)
			}

			return successfulState, waitErr
		}
		mutex.Unlock()

		time.Sleep(1 * time.Second)
	}
}

type WaitTimeoutErr struct {
	machineID    string
	timeout      time.Duration
	desiredState string
}

func (e WaitTimeoutErr) Error() string {
	return "timeout reached waiting for machine's state to change"
}

func (e WaitTimeoutErr) Description() string {
	return fmt.Sprintf("The machine %s took more than %s to reach \"%s\"", e.machineID, e.timeout, e.desiredState)
}

func (e WaitTimeoutErr) DesiredState() string {
	return e.desiredState
}

var invalidAction flyerr.GenericErr = flyerr.GenericErr{
	Err:      "action must be either start or stop",
	Descript: "",
	Suggest:  "This is a bug in wait function, please report this at https://community.fly.io",
	DocUrl:   "",
}
