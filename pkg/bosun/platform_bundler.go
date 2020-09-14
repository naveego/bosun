package bosun

import (
	"archive/zip"
	"fmt"
	"github.com/mattn/go-zglob"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type PlatformBundler struct {
	p   *Platform
	b   *Bosun
	log *logrus.Entry
}

type BundlePlatformRequest struct {
	Dir          string
}

type BundlePlatformResult struct {
	Request *BundlePlatformRequest
	OutPath string
}

func NewPlatformBundler(bosun *Bosun, platform *Platform) PlatformBundler {
	return PlatformBundler{
		b: bosun,
		p: platform,
	}
}


func (d PlatformBundler) Execute() (BundlePlatformResult, error) {
	req := BundlePlatformRequest{}

	result := BundlePlatformResult{
		Request: &req,
	}

	if req.Dir == "" {
		req.Dir = d.p.FromPath
		req.Dir = filepath.Join(getDirIfFile(req.Dir))
		_ = os.MkdirAll(req.Dir, 0700)
	}

	d.log = d.b.NewContext().Log()

	fromDir := filepath.Dir(d.p.FromPath)
	releaseSrcDir := filepath.Join(fromDir, "releases", "current")
	appsSrcDir := filepath.Join(fromDir, "apps")

	tmpDir := filepath.Join(os.TempDir(), "bosun", "bundle")
	releaseDestDir := filepath.Join(tmpDir, "releases", "current")
	appsDestDir := filepath.Join(tmpDir, "apps")

	d.log.Infof("Using temp directory at %q", tmpDir)

	_ = os.RemoveAll(tmpDir)

	// copy charts
	err := os.MkdirAll(releaseSrcDir, 0700)
	if err != nil {
		return result, fmt.Errorf("could not create charts folder '%s': %w", releaseSrcDir, err)
	}
	err = copy.Copy(releaseSrcDir, releaseDestDir)
	if err != nil {
		return result, fmt.Errorf("could not copy charts: %w", err)
	}

	// copy apps
	err = os.MkdirAll(appsDestDir, 0700)
	if err != nil {
		return result, fmt.Errorf("could not create apps folder '%s': %w", appsDestDir, err)
	}
	err = copy.Copy(appsSrcDir, appsDestDir)
	if err != nil {
		return result, fmt.Errorf("could not copy apps: %w", err)
	}

	// we need to clear out the stuff before saving
	d.p.EnvironmentPaths = nil
	d.p.Apps = nil
	d.p.ZenHubConfig = nil

	platforms := map[string][]*Platform {
		"platforms": {d.p},
	}
	pBytes, err := yaml.Marshal(platforms)
	if err != nil {
		return result, fmt.Errorf("could not serialize platform: %w")
	}

	err = ioutil.WriteFile(filepath.Join(tmpDir, "platform.yaml"), pBytes, 0700)
	if err != nil {
		return result, fmt.Errorf("could not copy '%s' to '%s': %w", d.p.FromPath, tmpDir, err)
	}

	outPath := filepath.Join(req.Dir, "bundle.zip")

	tmpFiles, err := zglob.Glob(filepath.Join(tmpDir, "**/*"))
	if err != nil {
		panic(err)
	}
	d.log.Infof("Bundling %d files", len(tmpFiles))

	err = d.zipFiles(outPath, tmpDir, tmpFiles)

	if err != nil {
		return result, errors.WithStack(err)
	}

	result.OutPath = outPath

	return result, nil
}

// zipFiles compresses one or many files into a single zip archive file.
// Param 1: filename is the output zip file's name.
// Param 2: files is a list of files to add to the zip.
func (d PlatformBundler) zipFiles(filename string, basePath string, files []string) error {

	newZipFile, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer newZipFile.Close()

	zipWriter := zip.NewWriter(newZipFile)
	defer zipWriter.Close()

	// Add files to zip
	for _, file := range files {
		if err = d.addFileToZip(zipWriter, basePath, file); err != nil {
			return err
		}
	}

	return nil
}

func (d PlatformBundler) addFileToZip(zipWriter *zip.Writer, basePath string, filename string) error {

	fileToZip, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fileToZip.Close()

	// Get the file information
	info, err := fileToZip.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}

	// Using FileInfoHeader() above only uses the basename of the file. If we want
	// to preserve the folder structure we can overwrite this with the full path.
	header.Name = strings.TrimPrefix(filename, basePath)

	// Change to deflate to gain better compression
	// see http://golang.org/pkg/archive/zip/#pkg-constants
	header.Method = zip.Deflate

	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, fileToZip)
	return err
}
