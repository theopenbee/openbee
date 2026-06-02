package rpc

import "testing"

func TestNormalizeTaskExecutionLimit(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	cases := []struct {
		name         string
		raw          *int
		matched      int
		want         int
		wantErr      bool
	}{
		{"nil uses default", nil, 5, defaultTaskExecutionLimit, false},
		{"zero with single match", intPtr(0), 1, 0, false},
		{"zero with multi match errors", intPtr(0), 2, 0, true},
		{"negative clamps to zero", intPtr(-5), 3, 0, false},
		{"normal value passes through", intPtr(7), 3, 7, false},
		{"over max clamps to max", intPtr(maxTaskExecutionLimit + 50), 3, maxTaskExecutionLimit, false},
		{"exactly max passes through", intPtr(maxTaskExecutionLimit), 3, maxTaskExecutionLimit, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeTaskExecutionLimit(c.raw, c.matched)
			if (err != nil) != c.wantErr {
				t.Fatalf("err: got %v, wantErr=%v", err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("value: got %d, want %d", got, c.want)
			}
		})
	}
}
