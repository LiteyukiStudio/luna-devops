package worker

import (
	"testing"

	"github.com/LiteyukiStudio/devops/internal/tasks"
)

func TestPeriodicTaskSpecsIncludeNotificationReconcileEveryMinute(t *testing.T) {
	specs, err := periodicTaskSpecs()
	if err != nil {
		t.Fatalf("periodicTaskSpecs returned error: %v", err)
	}
	policy := tasks.PolicyForType(tasks.TypeNotificationReconcile)
	for _, spec := range specs {
		if spec.Task.Type() != tasks.TypeNotificationReconcile {
			continue
		}
		if spec.Cron != "@every 1m" || spec.Queue != policy.Queue || spec.Timeout != policy.Timeout ||
			spec.MaxRetry != policy.MaxRetry || spec.Retention != policy.Retention || spec.Unique != policy.Unique {
			t.Fatalf("notification reconcile spec = %#v, policy = %#v", spec, policy)
		}
		return
	}
	t.Fatal("notification reconcile periodic task is not registered")
}
