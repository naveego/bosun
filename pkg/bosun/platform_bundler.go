package bosun

import (
	"archive/zip"
	"github.com/mattn/go-zglob"
	"github.com/otiai10/copy"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io"
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
	fromDir = filepath.Join(fromDir, "releases", "current")

	tmpDir := filepath.Join(os.TempDir(), "bosun", "bundle")

	_ = os.RemoveAll(tmpDir)
	err := os.MkdirAll(tmpDir, 0700)
	if err != nil {
		return result, errors.WithStack(err)
	}

	d.log.Infof("Using temp directory at %q", tmpDir)

	err = copy.Copy(fromDir, tmpDir)
	if err != nil {
		return result, errors.Wrap(err, "making copying release files")
	}

	//// remove releases not requested
	//releaseDirs, _ := filepath.Glob(filepath.Join(tmpDir , "*"))
	//for _, releaseDir := range releaseDirs {
	//	slot := filepath.Base(releaseDir)
	//	for _, requestedSlot := range req.Releases {
	//		if slot == requestedSlot {
	//			d.log.Infof("Keeping release %q because it was requested", slot)
	//			goto FoundSlot
	//		}
	//	}
	//	d.log.Warnf("Discarding release %q because it was not requested", slot)
	//	_ = os.RemoveAll(releaseDir)
	//FoundSlot:
	//}

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
