module github.com/containerd/accelerated-container-image

go 1.15

require (
	github.com/containerd/console v1.0.2
	github.com/containerd/containerd v1.5.7
	github.com/containerd/containerd/api v1.5.0-beta.3 // indirect
	github.com/containerd/continuity v0.1.0
	github.com/containerd/go-cni v1.0.2
	github.com/deislabs/oras v0.2.1-alpha.1
	github.com/dgraph-io/ristretto v0.1.0
	github.com/elazarl/goproxy v0.0.0-20210801061803-8e322dfb79c4
	github.com/kr/text v0.2.0 // indirect
	github.com/moby/sys/mountinfo v0.4.1
	github.com/niemeyer/pretty v0.0.0-20200227124842-a10e7caefd8e // indirect
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.1
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/viper v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli v1.22.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20210630005230-0f9fa26af87c
	google.golang.org/grpc v1.38.0
	gopkg.in/check.v1 v1.0.0-20200227125254-8fa46927fb4f // indirect
	oras.land/oras-go v0.5.0 // indirect
)

replace (
	github.com/containerd/containerd => github.com/oras-project/containerd v1.5.0-beta.4.0.20210914182246-c90d5cff6817
	github.com/containerd/containerd/api => github.com/oras-project/containerd/api v0.0.0-20210914182246-c90d5cff6817
	github.com/deislabs/oras v0.2.1-alpha.1 => github.com/oras-project/oras v0.0.0-20211111051810-58fa344544b3
	github.com/elazarl/goproxy => github.com/taoting1234/goproxy v0.0.0-20210901033843-ebf581737889
	github.com/oras-project/artifacts-spec => github.com/oras-project/artifacts-spec v0.0.0-20211117234821-a732a936a0ff
)
