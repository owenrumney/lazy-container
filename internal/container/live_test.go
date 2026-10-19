//go:build integration

package container

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLiveLogs streams a few lines from the first running container.
// Run with: go test ./internal/container -tags integration -run TestLive -v
func TestLiveLogs(t *testing.T) {
	c, err := New()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := c.ListContainers(ctx)
	require.NoError(t, err)
	var id string
	for _, ctr := range containers {
		if ctr.Running() {
			id = ctr.ID
			break
		}
	}
	if id == "" {
		t.Skip("no running container")
	}

	lines, err := c.Logs(ctx, id, 5)
	require.NoError(t, err)
	n := 0
	for l := range lines {
		t.Logf("line: %.80s", l)
		if n++; n == 5 {
			cancel()
		}
	}
	require.Equal(t, 5, n)
}
