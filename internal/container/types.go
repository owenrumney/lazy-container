package container

import (
	"strings"
	"time"
)

// Container is one entry from `container ls --format json`.
type Container struct {
	ID            string                 `json:"id"`
	Configuration ContainerConfiguration `json:"configuration"`
	Status        ContainerStatus        `json:"status"`
}

type ContainerConfiguration struct {
	CreationDate   time.Time         `json:"creationDate"`
	Image          ImageRef          `json:"image"`
	Labels         map[string]string `json:"labels"`
	Platform       Platform          `json:"platform"`
	PublishedPorts []PublishedPort   `json:"publishedPorts"`
	Resources      Resources         `json:"resources"`
	Networks       []NetworkAttach   `json:"networks"`
	Mounts         []Mount           `json:"mounts"`
}

type ImageRef struct {
	Reference string `json:"reference"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Variant      string `json:"variant"`
}

type PublishedPort struct {
	HostAddress   string `json:"hostAddress"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Proto         string `json:"proto"`
}

type Resources struct {
	CPUs          int   `json:"cpus"`
	MemoryInBytes int64 `json:"memoryInBytes"`
}

type NetworkAttach struct {
	Network string            `json:"network"`
	Options map[string]string `json:"options"`
}

type Mount struct {
	Destination string `json:"destination"`
	Source      string `json:"source"`
}

type ContainerStatus struct {
	State       string          `json:"state"`
	StartedDate *time.Time      `json:"startedDate"`
	Networks    []NetworkStatus `json:"networks"`
}

type NetworkStatus struct {
	Network     string `json:"network"`
	Hostname    string `json:"hostname"`
	IPv4Address string `json:"ipv4Address"`
	IPv4Gateway string `json:"ipv4Gateway"`
}

// Image is one entry from `container image ls --format json`.
type Image struct {
	ID            string             `json:"id"`
	Configuration ImageConfiguration `json:"configuration"`
	Variants      []ImageVariant     `json:"variants"`
}

type ImageConfiguration struct {
	CreationDate time.Time  `json:"creationDate"`
	Name         string     `json:"name"`
	Descriptor   Descriptor `json:"descriptor"`
}

type Descriptor struct {
	Digest    string `json:"digest"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
}

type ImageVariant struct {
	Digest   string   `json:"digest"`
	Platform Platform `json:"platform"`
	Size     int64    `json:"size"`
}

// Volume is one entry from `container volume ls --format json`.
type Volume struct {
	ID            string              `json:"id"`
	Configuration VolumeConfiguration `json:"configuration"`
}

type VolumeConfiguration struct {
	CreationDate time.Time         `json:"creationDate"`
	Driver       string            `json:"driver"`
	Format       string            `json:"format"`
	Labels       map[string]string `json:"labels"`
	Name         string            `json:"name"`
	SizeInBytes  int64             `json:"sizeInBytes"`
	Source       string            `json:"source"`
}

// Network is one entry from `container network ls --format json`.
type Network struct {
	ID            string               `json:"id"`
	Configuration NetworkConfiguration `json:"configuration"`
	Status        NetworkStat          `json:"status"`
}

type NetworkConfiguration struct {
	CreationDate time.Time         `json:"creationDate"`
	Labels       map[string]string `json:"labels"`
	Mode         string            `json:"mode"`
	Name         string            `json:"name"`
	Plugin       string            `json:"plugin"`
}

type NetworkStat struct {
	IPv4Gateway string `json:"ipv4Gateway"`
	IPv4Subnet  string `json:"ipv4Subnet"`
	IPv6Subnet  string `json:"ipv6Subnet"`
}

// Stats is one entry from `container stats --no-stream --format json`.
type Stats struct {
	ID               string `json:"id"`
	BlockReadBytes   int64  `json:"blockReadBytes"`
	BlockWriteBytes  int64  `json:"blockWriteBytes"`
	CPUUsageUsec     int64  `json:"cpuUsageUsec"`
	MemoryLimitBytes int64  `json:"memoryLimitBytes"`
	MemoryUsageBytes int64  `json:"memoryUsageBytes"`
	NetworkRxBytes   int64  `json:"networkRxBytes"`
	NetworkTxBytes   int64  `json:"networkTxBytes"`
	NumProcesses     int    `json:"numProcesses"`
}

// DiskUsage is the output of `container system df --format json`.
type DiskUsage struct {
	Containers DiskUsageEntry `json:"containers"`
	Images     DiskUsageEntry `json:"images"`
	Volumes    DiskUsageEntry `json:"volumes"`
}

type DiskUsageEntry struct {
	Active      int   `json:"active"`
	Reclaimable int64 `json:"reclaimable"`
	SizeInBytes int64 `json:"sizeInBytes"`
	Total       int   `json:"total"`
}

// SystemStatus is parsed from the `container system status` table.
type SystemStatus struct {
	Running bool
	Version string
}

// Running reports whether the container is in the running state.
func (c Container) Running() bool {
	return c.Status.State == "running"
}

// Image returns the image reference without the docker.io/library/ noise.
func (c Container) Image() string {
	return ShortImageName(c.Configuration.Image.Reference)
}

// Name returns the repository part of the image reference, shortened.
func (i Image) Name() string {
	name, _ := splitTag(ShortImageName(i.Configuration.Name))
	return name
}

// Tag returns the tag part of the image reference, or "<none>".
func (i Image) Tag() string {
	_, tag := splitTag(i.Configuration.Name)
	if tag == "" {
		return "<none>"
	}
	return tag
}

// Size sums the sizes of all variants, matching the FULL SIZE column.
func (i Image) Size() int64 {
	var total int64
	for _, v := range i.Variants {
		total += v.Size
	}
	return total
}

// ShortID returns the first 12 characters of an ID or digest.
func ShortID(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// ShortImageName strips the implicit docker.io/library/ prefix.
func ShortImageName(ref string) string {
	ref = strings.TrimPrefix(ref, "docker.io/library/")
	return strings.TrimPrefix(ref, "docker.io/")
}

func splitTag(ref string) (name, tag string) {
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if colon > slash {
		return ref[:colon], ref[colon+1:]
	}
	return ref, ""
}
