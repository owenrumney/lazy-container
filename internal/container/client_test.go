package container

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func load[T any](t *testing.T, name string) T {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var v T
	require.NoError(t, json.Unmarshal(data, &v))
	return v
}

func TestParseContainers(t *testing.T) {
	containers := load[[]Container](t, "containers.json")
	require.Len(t, containers, 1)

	c := containers[0]
	assert.Equal(t, "buildkit", c.ID)
	assert.True(t, c.Running())
	assert.Equal(t, "ghcr.io/apple/container-builder-shim/builder:0.13.1", c.Image())
	assert.Equal(t, 2, c.Configuration.Resources.CPUs)
	assert.Equal(t, int64(2147483648), c.Configuration.Resources.MemoryInBytes)
	assert.Equal(t, "builder", c.Configuration.Labels["com.apple.container.plugin"])
	require.NotNil(t, c.Status.StartedDate)
	require.Len(t, c.Status.Networks, 1)
	assert.Equal(t, "192.168.64.2/24", c.Status.Networks[0].IPv4Address)
}

func TestParseImages(t *testing.T) {
	images := load[[]Image](t, "images.json")
	require.Len(t, images, 2)

	alpine := images[0]
	assert.Equal(t, "alpine", alpine.Name())
	assert.Equal(t, "latest", alpine.Tag())
	assert.Equal(t, int64(4184689), alpine.Size())
	assert.Equal(t, "28bd5fe8b56d", ShortID(alpine.ID))
}

func TestParseVolumes(t *testing.T) {
	volumes := load[[]Volume](t, "volumes.json")
	require.Len(t, volumes, 1)
	assert.Equal(t, "ctui-probe", volumes[0].Configuration.Name)
	assert.Equal(t, "ext4", volumes[0].Configuration.Format)
	assert.Equal(t, int64(549755813888), volumes[0].Configuration.SizeInBytes)
}

func TestParseNetworks(t *testing.T) {
	networks := load[[]Network](t, "networks.json")
	require.Len(t, networks, 1)
	assert.Equal(t, "default", networks[0].Configuration.Name)
	assert.Equal(t, "nat", networks[0].Configuration.Mode)
	assert.Equal(t, "192.168.64.0/24", networks[0].Status.IPv4Subnet)
}

func TestParseStats(t *testing.T) {
	stats := load[[]Stats](t, "stats.json")
	require.Len(t, stats, 1)
	assert.Equal(t, "buildkit", stats[0].ID)
	assert.Positive(t, stats[0].CPUUsageUsec)
	assert.Equal(t, 21, stats[0].NumProcesses)
}

func TestParseDiskUsage(t *testing.T) {
	df := load[DiskUsage](t, "df.json")
	assert.Equal(t, 8, df.Images.Total)
	assert.Equal(t, int64(20073508864), df.Images.Reclaimable)
	assert.Equal(t, 1, df.Containers.Active)
}

func TestParseSystemStatus(t *testing.T) {
	out := []byte(`FIELD              VALUE
status             running
appRoot            /Users/owen/Library/Application Support/com.apple.container/
apiserver.version  container-apiserver version 1.3.1 (build: release, commit: a9a62e2)
`)
	st := parseSystemStatus(out)
	assert.True(t, st.Running)
	assert.Equal(t, "1.3.1", st.Version)

	st = parseSystemStatus([]byte("status  stopped\n"))
	assert.False(t, st.Running)
	assert.Empty(t, st.Version)
}

func TestImageNameTag(t *testing.T) {
	tests := []struct {
		ref       string
		name, tag string
	}{
		{"docker.io/library/alpine:latest", "alpine", "latest"},
		{"docker.io/library/golang:1.26-bookworm", "golang", "1.26-bookworm"},
		{"ghcr.io/apple/containerization/vminit:0.41.0", "ghcr.io/apple/containerization/vminit", "0.41.0"},
		{"localhost:5000/foo", "localhost:5000/foo", "<none>"},
		{"alpine@sha256:abc", "alpine", "<none>"},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			img := Image{Configuration: ImageConfiguration{Name: tt.ref}}
			assert.Equal(t, tt.name, img.Name())
			assert.Equal(t, tt.tag, img.Tag())
		})
	}
}

func TestLastLine(t *testing.T) {
	assert.Equal(t, "container not found", lastLine("warning\nError: container not found\n\n"))
	assert.Empty(t, lastLine("  \n"))
}
