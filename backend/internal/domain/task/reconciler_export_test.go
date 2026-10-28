package task

import "context"

// ReconcileForTest runs a single reconcile pass synchronously.
func ReconcileForTest(r *Reconciler, ctx context.Context) {
	r.reconcile(ctx)
}

// SetProcessAliveForTest swaps the OS-level liveness probe.
func SetProcessAliveForTest(r *Reconciler, fn func(pid int) bool) {
	r.processAlive = fn
}
