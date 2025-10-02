package registry

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	crv1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pkg/errors"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/Harshit-Dhundale/helm-image-analyzer/internal/util"
)

type Info struct {
	Image       string
	ResolvedRef string
	Digest      string
	Layers      int
	SizeBytes   int64
	SizeHuman   string
	Platform    string
	Downloaded  string 
}


func InspectImage(ctx context.Context, image, platform string, download bool, downloadDir string) (Info, error) {
	ref, err := parseRef(image)
	if err != nil {
		return Info{}, errors.Wrap(err, "parse image")
	}

	osArch := strings.SplitN(platform, "/", 2)
	pl := &crv1.Platform{OS: "linux", Architecture: "amd64"}
	if len(osArch) == 2 && osArch[0] != "" && osArch[1] != "" {
		pl.OS = osArch[0]
		pl.Architecture = osArch[1]
	}


img, err := remote.Image(ref,
    remote.WithContext(ctx),
    remote.WithAuthFromKeychain(authn.DefaultKeychain),
    remote.WithPlatform(*pl), 
)

	if err != nil {
		return Info{}, errors.Wrap(err, "remote image")
	}

	digest, err := img.Digest()
	if err != nil {
		return Info{}, errors.Wrap(err, "digest")
	}

	manifest, err := img.Manifest()
	if err != nil {
		return Info{}, errors.Wrap(err, "manifest")
	}

	var total int64 = 0
	for _, l := range manifest.Layers {
		total += l.Size
	}
	total += manifest.Config.Size
	layers := len(manifest.Layers)

downloaded := ""
if download {
    dir := downloadDir
    if strings.TrimSpace(dir) == "" {
        var err error
        dir, err = os.MkdirTemp("", "images-*")
        if err != nil { return Info{}, errors.Wrap(err, "mktemp") }
    } else {
        if err := os.MkdirAll(dir, 0o755); err != nil {
            return Info{}, errors.Wrap(err, "mkdir download_dir")
        }
    }

    base := strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(ref.Name())
    file := filepath.Join(dir, base + ".tar")


   if err := tarball.WriteToFile(file, ref, img); err != nil {
        return Info{}, errors.Wrap(err, "tarball write")
    }
    downloaded = file
}


	return Info{
		Image:       image,
		ResolvedRef: ref.Name(),
		Digest:      digest.String(),
		Layers:      layers,
		SizeBytes:   total,
		SizeHuman:   util.HumanBytes(total),
		Platform:    fmt.Sprintf("%s/%s", pl.OS, pl.Architecture),
		Downloaded:  downloaded,
	}, nil
}

func parseRef(s string) (name.Reference, error) {

	ref, err := name.ParseReference(s, name.WithDefaultRegistry("index.docker.io"), name.WithDefaultTag("latest"))
	if err == nil {
		return ref, nil
	}

	if !strings.Contains(s, "/") {
		s = "library/" + s
	}
	return name.ParseReference("index.docker.io/" + s)
}
