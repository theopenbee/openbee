package store

import (
	"context"

	"github.com/theopenbee/openbee/desktop/internal/api"
)

func (s *Store) initOnboarding() {
	s.Onboarding = NewResource(s, func(ctx context.Context, c *api.Client) (api.OnboardingStatus, error) {
		return c.Onboarding(ctx)
	})
}

func (s *Store) OnboardingKnown() bool {
	return s.Onboarding.Loaded || s.Onboarding.Err != nil
}

func (s *Store) OnboardingPending() bool {
	return s.Onboarding.Loaded && s.Onboarding.Data.Pending
}

func (s *Store) FinishOnboarding() {
	s.Onboarding.Set(api.OnboardingStatus{})
}

func (s *Store) SkipOnboarding(done func(error)) {
	done = orNop(done)
	s.async(func(ctx context.Context, c *api.Client) func() {
		err := c.SkipOnboarding(ctx)
		return func() {
			if err == nil {
				s.FinishOnboarding()
			}
			done(err)
		}
	}, canceledBy(done))
}
