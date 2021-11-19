# OCI Artifact Conversion and Push

Github [link](https://github.com/akashsinghal/accelerated-container-image)

Currently, the DADI image that is produced after converting the original image using OverlayBD, is OCI spec compliant; However, using OCI Artifacts, the DADI image can be referenced by the original image.
There are a few primary motivations for using OCI Artifact:
1. The DADI Image's lifespan is directly attached to the original image it is derived from 
2. The DADI Image is easily discoverable using only the original image. See [ORAS discover](https://oras.land/cli/6_reference_types/#discovering-artifact-references)
    1. For consumers, this allows for more transparent lookup of DADI image and its contents
    2. For providers, this allows for a less invasive method for running accelerated images since the image to run at runtime is determined by the existence of an attached DADI Image artifact

## Proposed DADI conversion Process:

The current `obdconv` command for the custom `ctr` cli will be changed to add a `--push-artifact` flag along with required `--username` and `--password` fields for the target registry credentials. 

```
sudo bin/ctr obdconv --push-artifact --username <TARGET-REGISTRY-USERNAME> --password <TARGET-REGISTRY-PASSWORD> artifactstest.azurecr.io/teleport/redis:original artifactstest.azurecr.io/teleport/redis:oras
``` 

Assumptions:
- The referred image in the artifact manifest is the source image provided
- The specifed target image parameter specifies the artifact name and registry to push to

New Conversion Flow:
1. Existing process to use overlaybd to convert the image to DADI image format.
2. Generate the artifact spec by specifiying the `artifactType` and `mediaType`
3. Add the original manifest reference in the `subject` field
4. Add the `config` as the first entry in the `blobs` field
5. Add all the `layers` to the `blobs` field
6. Use ORAS go package Push function and provide `blobs` as the file field and pass in the other artifact spec entries as options.
7. ORAS will push to specified registry. 

## New Dependencies

- ORAS Go Module: github.com/deislabs/oras v0.2.1-alpha.1
- ORAS containerd: github.com/oras-project/containerd/api
- ORAS Artifact Spec: github.com/oras-project/artifacts-spec

## Performance

`obdconv` exection now takes ~ 16.62 seconds to complete for a redis image

## Converting from DADI OCI Image to DADI OCI Artifact

The contents of the artifact are the same as the original image. The only difference is the manifest. 

The `artifactType` field is an artifact specific field that can be used to specify artifact specs. Here we use `dadi.image.v1` as the artifact to distinguish the spec.

The `blob` field is equivalent to the `layers` field. In order to add the config field to the artifact, the standard convention for DADI image artifact is to add the config blob as the first blob in the list.

The `mediaType` must be `application/vnd.cncf.oras.artifact.manifest.v1+json`

the `subject` contains a reference to the original image manifest


### DADI Image Manifest -> DADI OCI Artifact Manifest

Here's the manifest lifecycle of the redis image:

Redis Image 
```
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "size": 7698,
    "digest": "sha256:de974760ddb2f32dbddb74b7bb8cff4c1eee06d43d36d11bbca1dc815173916e"
  },
  "layers": [
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 27139373,
      "digest": "sha256:f7ec5a41d630a33a2d1db59b95d89d93de7ae5a619a3a8571b78457e48266eba"
    },
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 1730,
      "digest": "sha256:a36224ca8bbdc65bae63504c0aefe0881e4ebdf1ec362830b347a2ab5120a8de"
    },
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 1417921,
      "digest": "sha256:7630ad34dcb2a98e15daa425b6e076c06f9f810c7714803a3bdbcf750a546ea0"
    },
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 10097197,
      "digest": "sha256:dd0ea236b03be5127b0b7fb44f1d39f757c5381a88d650fae0a0d80b0c493020"
    },
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 133,
      "digest": "sha256:ed6ed4f2f5a63e3428ddd456805d71dc55f68b763ae8d5350f29b8cea46373f2"
    },
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 409,
      "digest": "sha256:8788804112c6c0d3e91725e26d741c9b8e4ad910e63136e129043220eb8d11d4"
    }
  ]
}
```

DADI OCI Image Manifest
```
{
  "schemaVersion": 2,
  "config": {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "digest": "sha256:f873ef3cc686502ddeaeae009da21960cdb027847ceeb2605d0b7560aba85413",
      "size": 3445
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:a8b5fca80efae55088290f3da8110d7742de55c2a378d5ab53226a483f390e21",
      "size": 4739584,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:a8b5fca80efae55088290f3da8110d7742de55c2a378d5ab53226a483f390e21",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "4739584"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:cab750ab5436f7f22b18314a15ced78a8375c57e0ffeb1409dae5408273deb5e",
      "size": 43513344,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:cab750ab5436f7f22b18314a15ced78a8375c57e0ffeb1409dae5408273deb5e",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "43513344"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:f9aa97b3fac30f61648e744ecef6512223f6b321c270ce47b9032f9d403e789c",
      "size": 27136,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:f9aa97b3fac30f61648e744ecef6512223f6b321c270ce47b9032f9d403e789c",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "27136"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:387c9cba0a40b6dba603472a0d9d69315e6b066cd456c4a25670c20ff7bf3f89",
      "size": 2614272,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:387c9cba0a40b6dba603472a0d9d69315e6b066cd456c4a25670c20ff7bf3f89",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "2614272"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:b5c32659aad8da248d33c139f458712b3350fa58706170009f893f7cb7f7e75b",
      "size": 17311744,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:b5c32659aad8da248d33c139f458712b3350fa58706170009f893f7cb7f7e75b",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "17311744"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:d3cfb88c2b81025caa87d97783a9affae69f84b963ff27a5746afdc70d65c39e",
      "size": 8192,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:d3cfb88c2b81025caa87d97783a9affae69f84b963ff27a5746afdc70d65c39e",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "8192"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:af4673f4ecad6e2c00e943522ca06dbbe59f00b92e018f7e83fc6de95d8dea8f",
      "size": 12288,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:af4673f4ecad6e2c00e943522ca06dbbe59f00b92e018f7e83fc6de95d8dea8f",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "12288"
      }
    }
  ],
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json"
}
```

DADI OCI Artifact Manifest
```
{
  "mediaType": "application/vnd.cncf.oras.artifact.manifest.v1+json",
  "artifactType": "dadi.image.v1",
  "blobs": [
    {
      "mediaType": "application/vnd.docker.container.image.v1+json",
      "digest": "sha256:f873ef3cc686502ddeaeae009da21960cdb027847ceeb2605d0b7560aba85413",
      "size": 3445
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:a8b5fca80efae55088290f3da8110d7742de55c2a378d5ab53226a483f390e21",
      "size": 4739584,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:a8b5fca80efae55088290f3da8110d7742de55c2a378d5ab53226a483f390e21",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "4739584"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:cab750ab5436f7f22b18314a15ced78a8375c57e0ffeb1409dae5408273deb5e",
      "size": 43513344,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:cab750ab5436f7f22b18314a15ced78a8375c57e0ffeb1409dae5408273deb5e",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "43513344"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:f9aa97b3fac30f61648e744ecef6512223f6b321c270ce47b9032f9d403e789c",
      "size": 27136,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:f9aa97b3fac30f61648e744ecef6512223f6b321c270ce47b9032f9d403e789c",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "27136"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:387c9cba0a40b6dba603472a0d9d69315e6b066cd456c4a25670c20ff7bf3f89",
      "size": 2614272,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:387c9cba0a40b6dba603472a0d9d69315e6b066cd456c4a25670c20ff7bf3f89",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "2614272"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:b5c32659aad8da248d33c139f458712b3350fa58706170009f893f7cb7f7e75b",
      "size": 17311744,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:b5c32659aad8da248d33c139f458712b3350fa58706170009f893f7cb7f7e75b",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "17311744"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:d3cfb88c2b81025caa87d97783a9affae69f84b963ff27a5746afdc70d65c39e",
      "size": 8192,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:d3cfb88c2b81025caa87d97783a9affae69f84b963ff27a5746afdc70d65c39e",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "8192"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar",
      "digest": "sha256:af4673f4ecad6e2c00e943522ca06dbbe59f00b92e018f7e83fc6de95d8dea8f",
      "size": 12288,
      "annotations": {
        "containerd.io/snapshot/overlaybd/blob-digest": "sha256:af4673f4ecad6e2c00e943522ca06dbbe59f00b92e018f7e83fc6de95d8dea8f",
        "containerd.io/snapshot/overlaybd/blob-fs-type": "ext4",
        "containerd.io/snapshot/overlaybd/blob-size": "12288"
      }
    }
  ],
  "subject": {
    "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
    "digest": "sha256:820582b05253c2b968442b8af31d791ae64478bcc18e04826c5ce42f974d3272",
    "size": 1574
  }
}
```