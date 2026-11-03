package platform_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenbee/openbee/internal/platform"
)

type nopSender struct{ name string }

func (nopSender) Send(context.Context, platform.OutboundMessage) error { return nil }

func TestSenderRegistry_SetGetDelete(t *testing.T) {
	r := platform.NewSenderRegistry()
	_, ok := r.Get("telegram")
	assert.False(t, ok)

	r.Set("telegram", nopSender{"a"})
	r.Set("discord", nopSender{"b"})
	got, ok := r.Get("telegram")
	require.True(t, ok)
	assert.Equal(t, nopSender{"a"}, got)
	assert.Equal(t, []string{"discord", "telegram"}, r.IDs())

	r.Set("telegram", nopSender{"c"})
	got, _ = r.Get("telegram")
	assert.Equal(t, nopSender{"c"}, got)

	r.Delete("telegram")
	_, ok = r.Get("telegram")
	assert.False(t, ok)
	assert.Equal(t, []string{"discord"}, r.IDs())
}

func TestSenderRegistry_ConcurrentAccess(t *testing.T) {
	r := platform.NewSenderRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				id := fmt.Sprintf("p%d", (i+j)%4)
				r.Set(id, nopSender{id})
				r.Delete(id)
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				r.Get("p1")
				r.IDs()
			}
		}()
	}
	wg.Wait()
}

func TestSenderMap_Get(t *testing.T) {
	var s platform.Senders = platform.SenderMap{"local": nopSender{"l"}}
	got, ok := s.Get("local")
	require.True(t, ok)
	assert.Equal(t, nopSender{"l"}, got)
	_, ok = s.Get("telegram")
	assert.False(t, ok)
}
