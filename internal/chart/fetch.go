package chart

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/pkg/errors"

	"github.com/Harshit-Dhundale/helm-image-analyzer/internal/util"
)

type Meta struct {
	Source  string
	Ref     string
	Subpath string
}

type Workdir struct {
	Root     string
	ChartDir string
}

type cleanupFunc func()


func FetchChart(ctx context.Context, ChartURL, LocalPath, Ref, Subpath string) (*Workdir, *Meta, cleanupFunc, error) {
	tmp, err := os.MkdirTemp("", "chart-work-*")
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "mktemp")
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	meta := &Meta{}
	work := &Workdir{Root: tmp}

	if LocalPath != "" {
		abs, err := filepath.Abs(LocalPath)
		if err != nil {
			cleanup()
			return nil, nil, nil, errors.Wrap(err, "abs local path")
		}
		meta.Source = abs
		meta.Ref = ""
		meta.Subpath = ""
		work.ChartDir = abs
		return work, meta, cleanup, nil
	}

	if ChartURL == "" {
		cleanup()
		return nil, nil, nil, errors.New("empty chart_url")
	}


	if strings.Contains(ChartURL, "github.com/") && strings.Contains(ChartURL, "/tree/") {
		u, _ := url.Parse(ChartURL)
		parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")

		if len(parts) < 4 || parts[2] != "tree" {
			cleanup()
			return nil, nil, nil, errors.New("unrecognized GitHub tree URL")
		}
		org, repo := parts[0], parts[1]
		ref := parts[3]
		sp := ""
		if len(parts) > 4 {
			sp = path.Join(parts[4:]...)
		}
		repoURL := fmt.Sprintf("https://github.com/%s/%s.git", org, repo)
		dir := filepath.Join(tmp, "repo")
		if _, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
			URL:           repoURL,
			SingleBranch:  true,
			Depth:         1,
			ReferenceName: util.RefToRefName(ref),
		}); err != nil {
			cleanup()
			return nil, nil, nil, errors.Wrap(err, "clone github repo")
		}
		work.ChartDir = filepath.Join(dir, sp)
		meta.Source = ChartURL
		meta.Ref = ref
		meta.Subpath = sp
		return work, meta, cleanup, nil
	}


	if strings.HasSuffix(strings.ToLower(ChartURL), ".tgz") {
		dst := filepath.Join(tmp, "chart.tgz")
		if err := download(ctx, ChartURL, dst); err != nil {
			cleanup()
			return nil, nil, nil, errors.Wrap(err, "download tgz")
		}
		extractDir := filepath.Join(tmp, "extracted")
		if err := untarGz(dst, extractDir); err != nil {
			cleanup()
			return nil, nil, nil, errors.Wrap(err, "untar")
		}

		cd, err := findChartRoot(extractDir, Subpath)
		if err != nil {
			cleanup()
			return nil, nil, nil, err
		}
		work.ChartDir = cd
		meta.Source = ChartURL
		meta.Subpath = strings.TrimPrefix(strings.TrimPrefix(cd, extractDir), string(os.PathSeparator))
		meta.Ref = Ref
		return work, meta, cleanup, nil
	}


	if isGitURL(ChartURL) || strings.Contains(ChartURL, "github.com/") {
		dir := filepath.Join(tmp, "repo")
		if _, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
    URL:           ChartURL,
    SingleBranch:  Ref != "",
    Depth:         1,
    ReferenceName: util.RefToRefName(Ref), 
}); err != nil {
    cleanup()
    return nil, nil, nil, errors.Wrap(err, "clone repo")
}

		cd, err := findChartRoot(dir, Subpath)
		if err != nil {
			cleanup()
			return nil, nil, nil, err
		}
		work.ChartDir = cd
		meta.Source = ChartURL
		meta.Ref = Ref
		meta.Subpath = Subpath
		return work, meta, cleanup, nil
	}

	return nil, nil, nil, errors.New("unsupported chart_url format")
}

func isGitURL(s string) bool {
	return strings.HasSuffix(s, ".git") ||
		strings.HasPrefix(s, "git@") ||
		strings.HasPrefix(s, "ssh://") ||
		strings.HasPrefix(s, "git+ssh://") ||
		strings.HasPrefix(s, "git+https://")
}

func download(ctx context.Context, src, dst string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return errors.Errorf("http %d from %s", resp.StatusCode, src)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func untarGz(tgzPath, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, h.Name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func findChartRoot(root, subpath string) (string, error) {
	if subpath != "" {
		cp := filepath.Join(root, subpath)
		if exists(filepath.Join(cp, "Chart.yaml")) {
			return cp, nil
		}
		return "", errors.Errorf("Chart.yaml not found at subpath %s", cp)
	}

	if exists(filepath.Join(root, "Chart.yaml")) {
		return root, nil
	}

	var found string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
		if d.IsDir() && exists(filepath.Join(p, "Chart.yaml")) && found == "" {
			found = p
			return io.EOF
		}
		return nil
	})
	if found == "" {
		return "", errors.New("could not locate Chart.yaml")
	}
	return found, nil
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }


