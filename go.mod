module github.com/containerd/accelerated-container-image

go 1.15

require (
	github.com/containerd/console v1.0.2
	github.com/containerd/containerd v1.5.8
	// github.com/containerd/containerd/api v0.0.0
	github.com/containerd/continuity v0.1.0
	github.com/containerd/go-cni v1.0.2
	github.com/kr/text v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.4.1
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/oras-project/artifacts-spec v1.0.0-draft.1.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	google.golang.org/grpc v1.38.0
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	oras.land/oras-go v1.0.0
)

replace (
	// github.com/containerd/containerd => github.com/oras-project/containerd v1.5.0-beta.4.0.20210914182246-c90d5cff6817
	// github.com/containerd/containerd/api => github.com/oras-project/containerd/api v0.0.0-20210914182246-c90d5cff6817
	github.com/elazarl/goproxy => github.com/taoting1234/goproxy v0.0.0-20210901033843-ebf581737889
	github.com/opencontainers/image-spec v1.0.2 => github.com/opencontainers/image-spec v1.0.1
// oras.land/oras-go v1.0.0 => github.com/oras-project/oras-go v0.0.0-20211118233813-9ec21342ac6c
)
