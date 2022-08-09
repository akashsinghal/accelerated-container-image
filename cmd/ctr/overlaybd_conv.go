/*
   Copyright The Accelerated Container Image Authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/archive/compression"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/log"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/snapshots"
	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"

	_ "github.com/go-sql-driver/mysql"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/identity"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	artifactspec "github.com/oras-project/artifacts-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"golang.org/x/sync/errgroup"

	"oras.land/oras-go/v2"
	ocitarget "oras.land/oras-go/v2/content/oci"
	registry "oras.land/oras-go/v2/registry/remote"
	orasauth "oras.land/oras-go/v2/registry/remote/auth"
)

const (
	labelOverlayBDBlobDigest   = "containerd.io/snapshot/overlaybd/blob-digest"
	labelOverlayBDBlobSize     = "containerd.io/snapshot/overlaybd/blob-size"
	labelOverlayBDBlobFsType   = "containerd.io/snapshot/overlaybd/blob-fs-type"
	labelOverlayBDBlobWritable = "containerd.io/snapshot/overlaybd.writable"
	labelKeyAccelerationLayer  = "containerd.io/snapshot/overlaybd/acceleration-layer"
	labelBuildLayerFrom        = "containerd.io/snapshot/overlaybd/build.layer-from"
	labelDistributionSource    = "containerd.io/distribution.source"
	contentStoreRootPath       = "/var/lib/containerd/io.containerd.content.v1.content"
	obdManifestPath            = "obd"
)

var (
	emptyString    string
	emptyDesc      ocispec.Descriptor
	emptyLayer     layer
	password       string
	username       string
	plainHTTP      bool
	insecure       bool
	authConfigPath string

	artifactTypeName       = "dadi.image.v1"
	convSnapshotNameFormat = "overlaybd-conv-%s"
	convLeaseNameFormat    = convSnapshotNameFormat
	convContentNameFormat  = convSnapshotNameFormat
)

type ImageConvertor interface {
	Convert(ctx context.Context, srcManifest ocispec.Manifest, fsType string) (ocispec.Descriptor, error)
}

var convertCommand = cli.Command{
	Name:        "obdconv",
	Usage:       "convert image layer into overlaybd format type",
	ArgsUsage:   "<src-image> <dst-image>",
	Description: `Export images to an OCI tar[.gz] into zfile format`,
	Flags: append(commands.RegistryFlags,
		cli.StringFlag{
			Name:  "fstype",
			Usage: "filesystem type(required), used to mount block device, support specifying mount options and mkfs options, separate fs type and options by ';', separate mount options by ',', separate mkfs options by ' '",
			Value: "ext4",
		},
		cli.StringFlag{
			Name:  "dbstr",
			Usage: "data base config string used for layer deduplication",
			Value: "",
		},
		cli.BoolFlag{
			Name:   "push-artifact",
			Usage:  "convert to OCI Artifact, add original manifest as referrer, and push to specified registry",
			Hidden: false,
		},
		cli.StringFlag{
			Name:  "obd-repository",
			Usage: "specify custom repository path appended after src-image for OCI DADI image pushed to registry. Default is 'obd'",
			Value: "",
		},
		cli.BoolFlag{
			Name:   "insecure",
			Usage:  "allow connections to SSL registry without certs",
			Hidden: false,
		},
		cli.StringFlag{
			Name:  "auth-config",
			Usage: "auth config path",
			Value: "",
		},
	),
	Action: func(context *cli.Context) error {
		var (
			srcImage    = context.Args().First()
			targetImage = context.Args().Get(1)
		)

		username = context.String("user")
		if i := strings.IndexByte(username, ':'); i > 0 {
			password = username[i+1:]
			username = username[0:i]
		}
		plainHTTP = context.Bool("plain-http")
		insecure = context.Bool("insecure")
		authConfigPath = context.String("auth-config")
		pushArtifact := context.Bool("push-artifact")
		obdRepository := context.String("obd-repository")

		if obdRepository == "" {
			obdRepository = obdManifestPath
		}

		if srcImage == "" {
			return errors.New("please provide src image name")
		}

		if pushArtifact && targetImage != "" {
			return errors.New("please only provide src image name for artifact convert")
		}

		if !pushArtifact && targetImage == "" {
			return errors.New("please provide dest image name")
		}

		cli, ctx, cancel, err := commands.NewClient(context)
		if err != nil {
			return err
		}
		defer cancel()

		ctx, done, err := cli.WithLease(ctx,
			leases.WithID(fmt.Sprintf(convLeaseNameFormat, uniquePart())),
			leases.WithExpiration(1*time.Hour),
		)
		if err != nil {
			return errors.Wrap(err, "failed to create lease")
		}
		defer done(ctx)

		var (
			sn = cli.SnapshotService("overlaybd")
			cs = cli.ContentStore()
		)

		fsType := context.String("fstype")
		fmt.Printf("filesystem type: %s\n", fsType)
		dbstr := context.String("dbstr")
		if dbstr != "" {
			fmt.Printf("database config string: %s\n", dbstr)
		}

		srcImg, err := ensureImageExist(ctx, cli, srcImage)
		if err != nil {
			return err
		}

		srcManifest, err := currentPlatformManifest(ctx, cs, srcImg)
		if err != nil {
			return errors.Wrapf(err, "failed to read manifest")
		}

		resolver, err := commands.GetResolver(ctx, context)
		if err != nil {
			return err
		}

		c, err := NewOverlaybdConvertor(ctx, cs, sn, resolver, targetImage, dbstr)
		if err != nil {
			return err
		}
		newMfstDesc, err := c.Convert(ctx, srcManifest, fsType)
		if err != nil {
			return err
		}

		parseResult, err := reference.Parse(srcImage)
		if err != nil {
			return fmt.Errorf("failed to parse src img reference %v", err)
		}

		destDadiImage := fmt.Sprintf("%s/%s:%s-%s", parseResult.Locator, obdRepository, newMfstDesc.Digest.Algorithm().String(), newMfstDesc.Digest.Encoded())
		destArtifactRef := parseResult.Locator

		if targetImage == "" {
			targetImage = destDadiImage
		}

		newImage := images.Image{
			Name:   targetImage,
			Target: newMfstDesc,
		}
		err = createImage(ctx, cli.ImageService(), newImage)
		if err != nil {
			return err
		}
		if pushArtifact {
			newImageManifest, err := images.Manifest(ctx, cs, newMfstDesc, platforms.Default())
			if err != nil {
				return err
			}
			// if manifest list or OCI index, must get descriptor of appropriate platform to use to attach artifact to
			srcManifestDesc := srcImg.Target()
			if srcManifestDesc.MediaType == images.MediaTypeDockerSchema2ManifestList || srcManifestDesc.MediaType == ocispec.MediaTypeImageIndex {
				fmt.Println("image of type index found")
				manifests, err := images.Children(ctx, cs, srcManifestDesc)
				if err != nil {
					fmt.Println("failed at to get manifests in index: Line 219")
					return err
				}
				platformComparator := platforms.Default()
				for _, desc := range manifests {
					if desc.Platform != nil && platformComparator.Match(*desc.Platform) {
						fmt.Printf("found matching manifest to platform: %v\n", desc.Digest)
						srcManifestDesc = desc
						break
					}
				}
			}
			return prepareArtifactAndPush(ctx, cs, srcManifestDesc, newImageManifest, destArtifactRef, cli.ImageService())
		}
		return nil
	},
}

type layer struct {
	desc   ocispec.Descriptor
	diffID digest.Digest
}

// contentLoader can load multiple files into content.Store service, and return an oci.v1.tar layer.
func newContentLoaderWithFsType(isAccelLayer bool, fsType string, files ...contentFile) *contentLoader {
	return &contentLoader{
		files:        files,
		isAccelLayer: isAccelLayer,
		fsType:       fsType,
	}
}

type contentFile struct {
	srcFilePath string
	dstFileName string
}

type contentLoader struct {
	files        []contentFile
	isAccelLayer bool
	fsType       string
}

func (loader *contentLoader) Load(ctx context.Context, cs content.Store) (l layer, err error) {
	refName := fmt.Sprintf(convContentNameFormat, uniquePart())
	contentWriter, err := content.OpenWriter(ctx, cs, content.WithRef(refName))
	if err != nil {
		return emptyLayer, errors.Wrapf(err, "failed to open content writer")
	}
	defer contentWriter.Close()

	srcPathList := make([]string, 0)
	digester := digest.Canonical.Digester()
	countWriter := &writeCountWrapper{w: io.MultiWriter(contentWriter, digester.Hash())}
	tarWriter := tar.NewWriter(countWriter)

	openedSrcFile := make([]*os.File, 0)
	defer func() {
		for _, each := range openedSrcFile {
			_ = each.Close()
		}
	}()

	for _, loader := range loader.files {
		srcPathList = append(srcPathList, loader.srcFilePath)
		srcFile, err := os.Open(loader.srcFilePath)
		if err != nil {
			return emptyLayer, errors.Wrapf(err, "failed to open src file of %s", loader.srcFilePath)
		}
		openedSrcFile = append(openedSrcFile, srcFile)

		fi, err := os.Stat(loader.srcFilePath)
		if err != nil {
			return emptyLayer, errors.Wrapf(err, "failed to get info of %s", loader.srcFilePath)
		}

		if err := tarWriter.WriteHeader(&tar.Header{
			Name:     loader.dstFileName,
			Mode:     0444,
			Size:     fi.Size(),
			Typeflag: tar.TypeReg,
		}); err != nil {
			return emptyLayer, errors.Wrapf(err, "failed to write tar header")
		}

		if _, err := io.Copy(tarWriter, bufio.NewReader(srcFile)); err != nil {
			return emptyLayer, errors.Wrapf(err, "failed to copy IO")
		}
	}

	if err = tarWriter.Close(); err != nil {
		return emptyLayer, errors.Wrapf(err, "failed to close tar file")
	}

	labels := map[string]string{
		labelBuildLayerFrom: strings.Join(srcPathList, ","),
	}

	if err := contentWriter.Commit(ctx, countWriter.c, digester.Digest(), content.WithLabels(labels)); err != nil {
		if !errdefs.IsAlreadyExists(err) {
			return emptyLayer, errors.Wrapf(err, "failed to commit content")
		}
	}

	l = layer{
		desc: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageLayer,
			Digest:    digester.Digest(),
			Size:      countWriter.c,
			Annotations: map[string]string{
				labelOverlayBDBlobDigest: digester.Digest().String(),
				labelOverlayBDBlobSize:   fmt.Sprintf("%d", countWriter.c),
			},
		},
		diffID: digester.Digest(),
	}
	if loader.isAccelLayer {
		l.desc.Annotations[labelKeyAccelerationLayer] = "yes"
	}
	if loader.fsType != "" {
		l.desc.Annotations[labelOverlayBDBlobFsType] = loader.fsType
	}
	return l, nil
}

type overlaybdConvertor struct {
	ImageConvertor
	cs      content.Store
	sn      snapshots.Snapshotter
	remote  bool
	fetcher remotes.Fetcher
	pusher  remotes.Pusher
	db      *sql.DB
	host    string
	repo    string
}

func NewOverlaybdConvertor(ctx context.Context, cs content.Store, sn snapshots.Snapshotter, resolver remotes.Resolver, ref string, dbstr string) (ImageConvertor, error) {
	c := &overlaybdConvertor{
		cs:     cs,
		sn:     sn,
		remote: false,
	}
	var err error
	if dbstr != "" {
		c.remote = true
		c.db, err = sql.Open("mysql", dbstr)
		if err != nil {
			return nil, err
		}
		c.pusher, err = resolver.Pusher(ctx, ref)
		if err != nil {
			return nil, err
		}
		c.fetcher, err = resolver.Fetcher(ctx, ref)
		if err != nil {
			return nil, err
		}
		refspec, err := reference.Parse(ref)
		if err != nil {
			return nil, err
		}
		c.host = refspec.Hostname()
		c.repo = strings.TrimPrefix(refspec.Locator, c.host+"/")
	}
	return c, nil
}

func (c *overlaybdConvertor) Convert(ctx context.Context, srcManifest ocispec.Manifest, fsType string) (ocispec.Descriptor, error) {
	configData, err := content.ReadBlob(ctx, c.cs, srcManifest.Config)
	if err != nil {
		return emptyDesc, err
	}

	var srcCfg ocispec.Image
	if err := json.Unmarshal(configData, &srcCfg); err != nil {
		return emptyDesc, err
	}

	committedLayers, err := c.convertLayers(ctx, srcManifest.Layers, srcCfg.RootFS.DiffIDs, fsType)
	if err != nil {
		return emptyDesc, err
	}

	return c.commitImage(ctx, srcManifest, srcCfg, committedLayers)
}

func (c *overlaybdConvertor) commitImage(ctx context.Context, srcManifest ocispec.Manifest, imgCfg ocispec.Image, committedLayers []layer) (ocispec.Descriptor, error) {
	var copyManifest = struct {
		ocispec.Manifest `json:",omitempty"`
		// MediaType is the media type of the object this schema refers to.
		MediaType string `json:"mediaType,omitempty"`
	}{
		Manifest:  srcManifest,
		MediaType: images.MediaTypeDockerSchema2Manifest,
	}

	imgCfg.RootFS.DiffIDs = nil
	copyManifest.Layers = nil

	for _, l := range committedLayers {
		copyManifest.Layers = append(copyManifest.Layers, l.desc)
		imgCfg.RootFS.DiffIDs = append(imgCfg.RootFS.DiffIDs, l.diffID)
	}

	configData, err := json.MarshalIndent(imgCfg, "", "   ")
	if err != nil {
		return emptyDesc, errors.Wrap(err, "failed to marshal image")
	}

	config := ocispec.Descriptor{
		MediaType: srcManifest.Config.MediaType,
		Digest:    digest.Canonical.FromBytes(configData),
		Size:      int64(len(configData)),
	}

	ref := remotes.MakeRefKey(ctx, config)
	if err := content.WriteBlob(ctx, c.cs, ref, bytes.NewReader(configData), config); err != nil {
		return ocispec.Descriptor{}, errors.Wrap(err, "failed to write image config")
	}
	if c.remote {
		if err := c.pushObject(ctx, config); err != nil {
			return ocispec.Descriptor{}, errors.Wrap(err, "failed to push image config")
		}
		log.G(ctx).Infof("config pushed")
	}

	copyManifest.Manifest.Config = config
	mb, err := json.MarshalIndent(copyManifest, "", "   ")
	if err != nil {
		return emptyDesc, err
	}

	desc := ocispec.Descriptor{
		MediaType: copyManifest.MediaType,
		Digest:    digest.Canonical.FromBytes(mb),
		Size:      int64(len(mb)),
	}

	labels := map[string]string{}
	labels["containerd.io/gc.ref.content.config"] = copyManifest.Config.Digest.String()
	for i, ch := range copyManifest.Layers {
		labels[fmt.Sprintf("containerd.io/gc.ref.content.l.%d", i)] = ch.Digest.String()
	}

	ref = remotes.MakeRefKey(ctx, desc)
	if err := content.WriteBlob(ctx, c.cs, ref, bytes.NewReader(mb), desc, content.WithLabels(labels)); err != nil {
		return emptyDesc, errors.Wrap(err, "failed to write image manifest")
	}
	if c.remote {
		if err := c.pushObject(ctx, desc); err != nil {
			return ocispec.Descriptor{}, errors.Wrap(err, "failed to push image manifest")
		}
		log.G(ctx).Infof("image pushed")
	}
	return desc, nil
}

type OverlaybdLayer struct {
	Host       string
	Repo       string
	ChainID    string
	DataDigest string
	DataSize   int64
}

func (c *overlaybdConvertor) findRemote(ctx context.Context, chainID string) (ocispec.Descriptor, error) {
	row := c.db.QueryRow("select host, repo, chain_id, data_digest, data_size from overlaybd_layers where host=? and repo=? and chain_id=?", c.host, c.repo, chainID)
	// try to find in the same repo, check existence on registry
	var layer OverlaybdLayer
	if err := row.Scan(&layer.Host, &layer.Repo, &layer.ChainID, &layer.DataDigest, &layer.DataSize); err == nil {
		desc := ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageLayer,
			Digest:    digest.Digest(layer.DataDigest),
			Size:      layer.DataSize,
		}
		rc, err := c.fetcher.Fetch(ctx, desc)
		if err == nil {
			rc.Close()
			log.G(ctx).Infof("found remote layer for chainID %s", chainID)
			return desc, nil
		}
		if errdefs.IsNotFound(err) {
			// invalid record in db, which is not found in registry, remove it
			_, err := c.db.Exec("delete from overlaybd_layers where host=? and repo=? and chain_id=?", c.host, c.repo, chainID)
			if err != nil {
				return emptyDesc, errors.Wrapf(err, "failed to remove invalid record in db")
			}
		}
	}

	// found record in other repo, mount it to target repo
	rows, err := c.db.Query("select host, repo, chain_id, data_digest, data_size from overlaybd_layers where host=? and chain_id=?", c.host, chainID)
	if err != nil {
		if err == sql.ErrNoRows {
			return emptyDesc, errdefs.ErrNotFound
		}
		log.G(ctx).Infof("query error %v", err)
		return emptyDesc, err
	}
	for rows.Next() {
		var layer OverlaybdLayer
		err = rows.Scan(&layer.Host, &layer.Repo, &layer.ChainID, &layer.DataDigest, &layer.DataSize)
		if err != nil {
			continue
		}
		// try mount
		desc := ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageLayer,
			Digest:    digest.Digest(layer.DataDigest),
			Size:      layer.DataSize,
			Annotations: map[string]string{
				fmt.Sprintf("%s.%s", labelDistributionSource, c.host): layer.Repo,
			},
		}
		_, err := c.pusher.Push(ctx, desc)
		if errdefs.IsAlreadyExists(err) {
			desc.Annotations = nil
			_, err := c.db.Exec("insert into overlaybd_layers(host, repo, chain_id, data_digest, data_size) values(?, ?, ?, ?, ?)", c.host, c.repo, chainID, desc.Digest.String(), desc.Size)
			if err != nil {
				continue
			}
			log.G(ctx).Infof("mount from %s success", layer.Repo)
			log.G(ctx).Infof("found remote layer for chainID %s", chainID)
			return desc, nil
		}
	}
	log.G(ctx).Infof("layer not found in remote")
	return emptyDesc, errdefs.ErrNotFound
}

func (c *overlaybdConvertor) pushObject(ctx context.Context, desc ocispec.Descriptor) error {
	ra, err := c.cs.ReaderAt(ctx, desc)
	if err != nil {
		return err
	}
	defer ra.Close()

	cw, err := c.pusher.Push(ctx, desc)
	if err != nil {
		if errdefs.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return content.Copy(ctx, cw, content.NewReader(ra), desc.Size, desc.Digest)
}

func (c *overlaybdConvertor) sentToRemote(ctx context.Context, desc ocispec.Descriptor, chainID string) error {
	// upload to registry
	err := c.pushObject(ctx, desc)
	if err != nil {
		return err
	}
	// update db
	_, err = c.db.Exec("insert into overlaybd_layers(host, repo, chain_id, data_digest, data_size) values(?, ?, ?, ?, ?)", c.host, c.repo, chainID, desc.Digest.String(), desc.Size)
	if err != nil {
		log.G(ctx).Warnf("failed to insert to db, err: %v", err)
		if strings.Contains(err.Error(), "Duplicate entry") {
			fmt.Printf("Conflict when inserting into db, maybe other process is converting the same blob, please try again later\n")
		}
		return err
	}
	return nil
}

// convertLayers applys image layers on overlaybd with specified filesystem and
// exports the layers based on zfile.
//
func (c *overlaybdConvertor) convertLayers(ctx context.Context, srcDescs []ocispec.Descriptor, srcDiffIDs []digest.Digest, fsType string) ([]layer, error) {
	var (
		lastParentID string = ""
		err          error
		commitLayers = make([]layer, len(srcDescs))
		chain        []digest.Digest
	)

	var sendToContentStore = func(ctx context.Context, snID string) (layer, error) {
		info, err := c.sn.Stat(ctx, snID)
		if err != nil {
			return emptyLayer, err
		}

		loader := newContentLoaderWithFsType(false, fsType, contentFile{
			info.Labels["containerd.io/snapshot/overlaybd.localcommitpath"],
			"overlaybd.commit"})
		return loader.Load(ctx, c.cs)
	}

	eg, ctx := errgroup.WithContext(ctx)
	for idx, desc := range srcDescs {
		chain = append(chain, srcDiffIDs[idx])
		chainID := identity.ChainID(chain).String()

		var remoteDesc ocispec.Descriptor

		if c.remote {
			remoteDesc, err = c.findRemote(ctx, chainID)
			if err != nil {
				if !errdefs.IsNotFound(err) {
					return nil, err
				}
			}
		}

		if c.remote && err == nil {
			key := fmt.Sprintf(convSnapshotNameFormat, chainID)
			opts := []snapshots.Opt{
				snapshots.WithLabels(map[string]string{
					"containerd.io/snapshot.ref":       key,
					"containerd.io/snapshot/image-ref": c.host + "/" + c.repo,
					labelOverlayBDBlobDigest:           remoteDesc.Digest.String(),
					labelOverlayBDBlobSize:             fmt.Sprintf("%d", remoteDesc.Size),
				}),
			}
			_, err = c.sn.Prepare(ctx, "prepare-"+key, lastParentID, opts...)
			if !errdefs.IsAlreadyExists(err) {
				// failed to prepare remote snapshot
				if err == nil {
					//rollback
					c.sn.Remove(ctx, "prepare-"+key)
				}
				return nil, errors.Wrapf(err, "failed to prepare remote snapshot")
			}
			lastParentID = key
			commitLayers[idx] = layer{
				desc: ocispec.Descriptor{
					MediaType: ocispec.MediaTypeImageLayer,
					Digest:    remoteDesc.Digest,
					Size:      remoteDesc.Size,
					Annotations: map[string]string{
						labelOverlayBDBlobDigest: remoteDesc.Digest.String(),
						labelOverlayBDBlobSize:   fmt.Sprintf("%d", remoteDesc.Size),
					},
				},
				diffID: remoteDesc.Digest,
			}
			continue
		}

		opts := []snapshots.Opt{
			snapshots.WithLabels(map[string]string{
				labelOverlayBDBlobWritable: "dir",
				labelOverlayBDBlobFsType:   fsType,
			}),
		}
		lastParentID, err = c.applyOCIV1LayerInZfile(ctx, lastParentID, desc, opts, nil)
		if err != nil {
			return nil, err
		}

		if c.remote {
			// must synchronize registry and db, can not do concurrently
			commitLayers[idx], err = sendToContentStore(ctx, lastParentID)
			if err != nil {
				return nil, err
			}
			err = c.sentToRemote(ctx, commitLayers[idx].desc, chainID)
			if err != nil {
				return nil, err
			}
		} else {
			idxI := idx
			snID := lastParentID
			eg.Go(func() error {
				var err error
				commitLayers[idxI], err = sendToContentStore(ctx, snID)
				return err
			})
		}
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return commitLayers, nil
}

// applyOCIV1LayerInZfile applys the OCIv1 tarfile in zfile format and commit it.
func (c *overlaybdConvertor) applyOCIV1LayerInZfile(
	ctx context.Context,
	parentID string, // the ID of parent snapshot
	desc ocispec.Descriptor, // the descriptor of layer
	snOpts []snapshots.Opt, // apply for the commit snapshotter
	afterApply func(root string) error, // do something after apply tar stream
) (string, error) {

	ra, err := c.cs.ReaderAt(ctx, desc)
	if err != nil {
		return emptyString, errors.Wrapf(err, "failed to get reader %s from content store", desc.Digest)
	}
	defer ra.Close()

	var (
		key    string
		mounts []mount.Mount
	)

	for {
		key = fmt.Sprintf(convSnapshotNameFormat, uniquePart())
		mounts, err = c.sn.Prepare(ctx, key, parentID, snOpts...)
		if err != nil {
			// retry other key
			if errdefs.IsAlreadyExists(err) {
				continue
			}
			return emptyString, errors.Wrapf(err, "failed to preprare snapshot %q", key)
		}

		break
	}

	var (
		rollback = true
		digester = digest.Canonical.Digester()
		rc       = io.TeeReader(content.NewReader(ra), digester.Hash())
	)

	defer func() {
		if rollback {
			if rerr := c.sn.Remove(ctx, key); rerr != nil {
				log.G(ctx).WithError(rerr).WithField("key", key).Warnf("apply failure and failed to cleanup snapshot")
			}
		}
	}()

	rc, err = compression.DecompressStream(rc)
	if err != nil {
		return emptyString, errors.Wrap(err, "failed to detect layer mediatype")
	}

	if err = mount.WithTempMount(ctx, mounts, func(root string) error {
		_, err := archive.Apply(ctx, root, rc)
		if err == nil && afterApply != nil {
			err = afterApply(root)
		}
		return err
	}); err != nil {
		return emptyString, errors.Wrapf(err, "failed to apply layer in snapshot %s", key)
	}

	// Read any trailing data
	if _, err := io.Copy(ioutil.Discard, rc); err != nil {
		return emptyString, err
	}

	commitID := fmt.Sprintf(convSnapshotNameFormat, digester.Digest())
	if err = c.sn.Commit(ctx, commitID, key); err != nil {
		if !errdefs.IsAlreadyExists(err) {
			return emptyString, err
		}
	}

	rollback = err != nil
	return commitID, nil
}

func ensureImageExist(ctx context.Context, cli *containerd.Client, imageName string) (containerd.Image, error) {
	return cli.GetImage(ctx, imageName)
}

func currentPlatformManifest(ctx context.Context, cs content.Provider, img containerd.Image) (ocispec.Manifest, error) {
	return images.Manifest(ctx, cs, img.Target(), platforms.Default())
}

func createImage(ctx context.Context, is images.Store, img images.Image) error {
	for {
		if _, err := is.Create(ctx, img); err != nil {
			if !errdefs.IsAlreadyExists(err) {
				return err
			}

			if _, err := is.Update(ctx, img); err != nil {
				if errdefs.IsNotFound(err) {
					continue
				}
				return err
			}
		}
		return nil
	}
}

// NOTE: based on https://github.com/containerd/containerd/blob/v1.4.3/rootfs/apply.go#L181-L187
func uniquePart() string {
	t := time.Now()
	var b [3]byte
	// Ignore read failures, just decreases uniqueness
	rand.Read(b[:])
	return fmt.Sprintf("%d-%s", t.Nanosecond(), strings.Replace(base64.URLEncoding.EncodeToString(b[:]), "_", "-", -1))
}

type writeCountWrapper struct {
	w io.Writer
	c int64
}

func (wc *writeCountWrapper) Write(p []byte) (n int, err error) {
	n, err = wc.w.Write(p)
	wc.c += int64(n)
	return
}

func prepareArtifactAndPush(ctx context.Context, cs content.Store, srcManifestDesc ocispec.Descriptor, obdManifest ocispec.Manifest, artifactTarget string, is images.Store) error {
	// create artifact
	var blobDesc []artifactspec.Descriptor

	// append the config as the first blob
	obdManifestConfig := obdManifest.Config
	blobDesc = append(blobDesc, artifactspec.Descriptor{
		MediaType:   obdManifestConfig.MediaType,
		Digest:      obdManifestConfig.Digest,
		Size:        obdManifestConfig.Size,
		URLs:        obdManifestConfig.URLs,
		Annotations: obdManifestConfig.Annotations,
	})
	for _, layer := range obdManifest.Layers {
		blobDesc = append(blobDesc, artifactspec.Descriptor{
			MediaType:   layer.MediaType,
			Digest:      layer.Digest,
			Size:        layer.Size,
			URLs:        layer.URLs,
			Annotations: layer.Annotations,
		})
	}
	artifactManifest := artifactspec.Manifest{
		MediaType:    "application/vnd.cncf.oras.artifact.manifest.v1+json",
		Blobs:        blobDesc,
		ArtifactType: artifactTypeName,
		Subject: artifactspec.Descriptor{
			MediaType:   srcManifestDesc.MediaType,
			Digest:      srcManifestDesc.Digest,
			Size:        srcManifestDesc.Size,
			URLs:        srcManifestDesc.URLs,
			Annotations: srcManifestDesc.Annotations,
		},
	}

	manifestBytes, err := json.MarshalIndent(artifactManifest, "", "   ")
	if err != nil {
		return err
	}
	manifestDescriptor := ocispec.Descriptor{
		MediaType: artifactspec.MediaTypeArtifactManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}

	refName := remotes.MakeRefKey(ctx, manifestDescriptor)

	// create the oras oci store
	artifactStore, err := ocitarget.New(contentStoreRootPath)
	if err != nil {
		fmt.Println("failed to create ORAS store: Line 872")
		return err
	}

	// store artifact manifest in content store
	// add garbage collection reference labels so blobs are not deleted
	labels := map[string]string{}
	labels["containerd.io/gc.root"] = string(manifestDescriptor.Digest)
	labels["containerd.io/gc.ref.content.config"] = obdManifestConfig.Digest.String()
	for i, ch := range obdManifest.Layers {
		labels[fmt.Sprintf("containerd.io/gc.ref.content.l.%d", i)] = ch.Digest.String()
	}
	err = content.WriteBlob(ctx, cs, refName, bytes.NewReader(manifestBytes), manifestDescriptor, content.WithLabels(labels))
	if err != nil {
		fmt.Printf("failed write artifact manifest blob to content store: %v (Line 886)\n", manifestDescriptor.Digest)
		return err
	}

	// tag the manifest with a ref
	err = artifactStore.Tag(ctx, manifestDescriptor, refName)
	if err != nil {
		fmt.Println("failed to tag new artifact manifest: Line 893")
		return err
	}

	// create Repository Target
	repository, err := registry.NewRepository(artifactTarget)
	if err != nil {
		fmt.Printf("failed to create new repository for target %s: Line 219\n", artifactTarget)
		return err
	}

	// set the Repository Client Credentials
	repositoryClient := &orasauth.Client{
		Header: http.Header{
			"User-Agent": {"overlaybd"},
		},
		Cache:      orasauth.DefaultCache,
		Credential: credentialProvider,
	}

	// set the TSLClientConfig for HTTP client if insecure set to true
	if insecure {
		repositoryClient.Client = http.DefaultClient
		repositoryClient.Client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	// set the PlainHTTP to true if specified
	repository.PlainHTTP = plainHTTP
	repository.Client = repositoryClient

	// copy the artifact manifest to remote Repository
	artifactTarget = fmt.Sprintf("%s@%s", artifactTarget, manifestDescriptor.Digest.String())
	returnedManifestDesc, err := oras.Copy(ctx, artifactStore, refName, repository, artifactTarget, oras.DefaultCopyOptions)
	if err != nil {
		fmt.Println("failed to push artifact to target: Line 930")
		return err
	}

	fmt.Println("Pushed", artifactTarget)
	fmt.Println("Digest:", returnedManifestDesc.Digest)
	return nil
}

func credentialProvider(ctx context.Context, registry string) (orasauth.Credential, error) {
	if username != "" || password != "" {
		return orasauth.Credential{
			Username: username,
			Password: password,
		}, nil
	}

	// use docker config file to extract registry credentials
	var cfg *configfile.ConfigFile
	if authConfigPath != "" {
		cfg = configfile.New(authConfigPath)
		if _, err := os.Stat(authConfigPath); err == nil {
			file, err := os.Open(authConfigPath)
			if err != nil {
				return orasauth.EmptyCredential, err
			}
			defer file.Close()
			if err := cfg.LoadFromReader(file); err != nil {
				return orasauth.EmptyCredential, err
			}
		} else {
			return orasauth.EmptyCredential, err
		}
	} else {
		// attempt to use docker config at default path
		var err error
		cfg, err = config.Load(config.Dir())
		if err != nil {
			return orasauth.EmptyCredential, nil
		}
	}

	dockerAuthConfig := cfg.AuthConfigs[registry]
	username = dockerAuthConfig.Username
	password = dockerAuthConfig.Password
	if username != "" || password != "" {
		return orasauth.Credential{
			Username: username,
			Password: password,
		}, nil
	}

	return orasauth.EmptyCredential, nil
}
