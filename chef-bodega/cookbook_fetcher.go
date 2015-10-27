package bodega

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"github.com/marpaia/chef-golang"
	"log"
	"net/http"
	_ "sync"
	"time"
)

type CookbookFetchFile struct {
	path     string
	url      string
	contents *bytes.Buffer
}

type CookbookFetch struct {
	Name     string
	Version  string
	Cookbook *chef.CookbookVersion
	Error    error
	SkipSSL  bool

	fetch  chan *CookbookFetchFile
	tar    chan *CookbookFetchFile
	output chan *bytes.Buffer
	client *http.Client
}

func (c *Bodega) NewCookbookTarballFetch(name, version string, cookbook *chef.CookbookVersion, skipSSL bool) *CookbookFetch {
	return &CookbookFetch{
		Name:     name,
		Version:  version,
		Cookbook: cookbook,
		fetch:    make(chan *CookbookFetchFile, 100),
		tar:      make(chan *CookbookFetchFile, 100),
		output:   make(chan *bytes.Buffer),
		SkipSSL:  skipSSL,
	}
}

func (f *CookbookFetch) Run() (*bytes.Reader, error) {
	// iterate through each file, fetching and then adding it to the tar
	// file

	// set up our http client
	if f.SkipSSL {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		f.client = &http.Client{Transport: transport}
	} else {
		f.client = &http.Client{}
	}

	// Create a handler to fetch files. Populates f.contents
	go f.fetchFiles()

	// Create a handler for fetched files, which will return us the tarball
	go f.tarUpFiles()

	// basically we have to iterate each file in the cookbook, adding them
	// into a tar as we get them. We then gzip that and send it along.
	sections := [][]struct{ chef.CookbookItem }{f.Cookbook.Files, f.Cookbook.Templates, f.Cookbook.Recipes, f.Cookbook.Attributes, f.Cookbook.Definitions, f.Cookbook.Libraries, f.Cookbook.Providers, f.Cookbook.Resources, f.Cookbook.RootFiles}

	for _, section := range sections {
		for _, file := range section {
			log.Printf("Cookbook file is %s - %s", file.Name, file.Path)

			ff := &CookbookFetchFile{
				path:     file.Path,
				url:      file.Url,
				contents: nil,
			}

			f.fetch <- ff
		}
	}

	close(f.fetch)

	// wait for something to be written on output
	buf, ok := <-f.output

	if !ok {
		// we must have errored
		return nil, f.Error
	}

	return bytes.NewReader(buf.Bytes()), nil
}

func (f *CookbookFetch) fetchFiles() {
	defer close(f.tar)

	for ff := range f.fetch {
		log.Printf("Fetch for %s", ff.path)
		buf, err := f.fetchFile(ff)

		if err != nil {
			log.Printf("Failed to fetch %s: %s", ff.path, err)
			f.Error = fmt.Errorf("Failed to fetch %s: %s", ff.path, err)
			close(f.output)
			break
		}

		ff.contents = buf
		f.tar <- ff
	}

	log.Printf("fetchFiles is done")
}

func (f *CookbookFetch) fetchFile(ff *CookbookFetchFile) (*bytes.Buffer, error) {
	// make an HTTP request for the file
	resp, err := f.client.Get(ff.url)

	if err != nil {
		return nil, fmt.Errorf("Error downloading file %s: %s", ff.path, err)
	}

	defer resp.Body.Close()

	// pull the results into a buffer
	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)

	if err != nil {
		return nil, fmt.Errorf("Error buffering file %s: %s", ff.path, err)
	}

	return buf, nil
}

func (f *CookbookFetch) tarUpFiles() {
	buf := new(bytes.Buffer)

	gw := gzip.NewWriter(buf)
	defer gw.Close()

	tw := tar.NewWriter(gw)

	now := time.Now()

	for ff := range f.tar {
		log.Printf("need to tar up %s", ff.path)
		hdr := &tar.Header{
			Name:    fmt.Sprintf("%s/%s", f.Name, ff.path),
			Size:    int64(len(ff.contents.Bytes())),
			Mode:    0644,
			ModTime: now,
		}

		if err := tw.WriteHeader(hdr); err != nil {
			log.Fatalln(err)
		}
		if _, err := tw.Write(ff.contents.Bytes()); err != nil {
			log.Fatalln(err)
		}
	}

	// Done tarring?
	if f.Error != nil {
		// we errored somewhere, so just return...
		return
	}

	if err := tw.Close(); err != nil {
		f.Error = err
		close(f.output)
		return
	}

	f.output <- buf
}
