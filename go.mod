module github.com/containerd/accelerated-container-image

go 1.16

require (
	github.com/containerd/containerd v1.6.4
	github.com/containerd/continuity v0.2.2
	github.com/containerd/go-cni v1.1.5
	github.com/go-sql-driver/mysql v1.6.0
	github.com/moby/sys/mountinfo v0.5.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417
	github.com/oras-project/artifacts-spec v1.0.0-draft.1.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/urfave/cli v1.22.2
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20211216021012-1d35b9e2eb4e
	google.golang.org/grpc v1.43.0
	oras.land/oras-go/v2 v2.0.0-20220616071421-319a41f91d61
)

replace (
	github.com/elazarl/goproxy => github.com/taoting1234/goproxy v0.0.0-20210901033843-ebf581737889
	github.com/opencontainers/image-spec v1.0.2 => github.com/opencontainers/image-spec v1.0.1
// oras.land/oras-go/pkg => /home/akashsinghal/go/pkg/mod/oras.land/oras-go/v2/pkg
// oras.land/oras-go/v2 => /home/akashsinghal/go/pkg/mod/oras.land/oras-go/v2
)
