package minimart

import (
	_ "archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	_ "github.com/ctdk/goiardi/cookbook"
	_ "github.com/go-chef/chef"
	"github.com/marpaia/chef-golang"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
    "crypto/rsa"
	"encoding/pem"
	"crypto/x509"
    "errors"
)

type ProxyCookbookVersion struct {
	LocationPath string            `json:"location_path"`
	LocationType string            `json:"location_type"`
	Dependencies map[string]string `json:"dependencies"`
	CookbookVersion     *chef.CookbookVersion     `json:"-"`
}

type UniverseHandler struct {
	chef *Minimart
}

type Minimart struct {
	baseURL string

	cookbooks  map[string]map[string]*ProxyCookbookVersion
	chefServer string
	client     *chef.Chef
	ticker     *time.Ticker
	tickerDone chan struct{}

	pollMutex sync.RWMutex
}

// Config is used to pass configuration options into the NewMinimart() constructor.
type Config struct {
    // URI to the Chef Server, with protocol and organization path (if required)
    ChefServer string
    // Path to the file containing the client PEM
    ChefPEM string
    // Name of the Chef client
    ChefClient string
    // Base URL to use in the universe endpoint
    BaseURL string
    // Disable SSL validation for the Chef server
    SkipSSL bool
}

func NewMinimart(config *Config) *Minimart {
	// read PEM file
	key, err := ioutil.ReadFile(config.ChefPEM)
	if err != nil {
		log.Fatal("Couldn't read key.pem: %v", err)
	}

    rsaKey, err := keyFromString(key)

    if err != nil {
        log.Fatal("Failed to parse key: %v", err)
    }

    client := &chef.Chef{
        Url:    config.ChefServer,
        Key:    rsaKey,
        UserId: config.ChefClient,
        SSLNoVerify: config.SkipSSL,
        Version: "11.6.0",
    }

	return &Minimart{
		baseURL:    config.BaseURL,
		chefServer: config.ChefServer,
		client:     client,
		ticker:     nil,
		tickerDone: make(chan struct{}),
	}
}

// keyFromString parses an RSA private key from a string
func keyFromString(key []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(key)
	if block == nil {
		return nil, fmt.Errorf("block size invalid for '%s'", string(key))
	}
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return rsaKey, nil
}

func (c *Minimart) NewUniverseHandler() *UniverseHandler {
	return &UniverseHandler{
		chef: c,
	}
}

func (c *Minimart) NewCookbookHandler() *CookbookHandler {
	return &CookbookHandler{
		chef: c,
	}
}

func (c *Minimart) Universe() ([]byte, error) {
	return json.Marshal(c.cookbooks)
}

func (c *Minimart) scrapeCookbooks() {
	// don't poll more than once at the same time
	c.pollMutex.Lock()
	defer c.pollMutex.Unlock()

    // We can't use chef.GetCookbooks() because we need to get all cookbook
    // versions. For now we're duplicating the logic below...
    all_versions, err := c.getAllCookbooks()

	if err != nil {
		log.Printf("Error fetching cookbooks: %v", err)
		return
	}

	var cookbooks = make(map[string]map[string]*ProxyCookbookVersion)

	for name, cvlist := range all_versions {
		log.Printf("got cookbook %v", name)

		versions := make(map[string]*ProxyCookbookVersion)
		for _, v := range cvlist.Versions {
			cv, ok, err := c.client.GetCookbookVersion(name, v.Version)

			if err != nil {
				log.Printf("error fetching cookbook version: %v", err)
				break
			}

            if !ok {
                log.Printf("Couldn't find cookbook version: %s/%s", name, v.Version)
                break
            }

			versions[v.Version] = &ProxyCookbookVersion{
				LocationPath: fmt.Sprintf("%s/cookbooks/%s/%s/download", c.baseURL, name, v.Version),
				LocationType: "supermarket",
				Dependencies: cv.Metadata.Dependencies,
				CookbookVersion:     cv,
			}

			log.Printf("got %v/%v", name, v.Version)
		}

		cookbooks[name] = versions
	}

	c.cookbooks = cookbooks
	log.Printf("Cookbook cache complete.")
}

func (c *Minimart) getAllCookbooks() (map[string]*chef.Cookbook, error) {
	resp, err := c.client.Get("cookbooks?num_versions=all")
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()

	cookbooks := map[string]*chef.Cookbook{}
	json.Unmarshal(body, &cookbooks)

	return cookbooks, nil
}

func (c *Minimart) PollForCookbooks(interval time.Duration) {
	if c.ticker != nil {
		log.Fatal("already polling...")
		return
	}

	// also fire a poll right now
	go c.scrapeCookbooks()

	c.ticker = time.NewTicker(interval)

	for {
		select {
		case <-c.ticker.C:
			// run a poll
			log.Printf("Should poll for cookbooks.")
		case <-c.tickerDone:
			log.Printf("Should stop polling.")
			c.ticker.Stop()
			return
		}
	}
}

func (c *Minimart) StopPollingForCookbooks() {
	c.tickerDone <- struct{}{}
}

func (c *Minimart) CreateCookbookVersionTarball(name, version string) (*bytes.Buffer, error) {
	// Make sure we know this version
	if c.cookbooks[name] == nil || c.cookbooks[name][version] == nil {
		return nil, fmt.Errorf("Cookbook version not found: %s/%s", name, version)
	}

	buf := new(bytes.Buffer)

	cookbook := c.cookbooks[name][version].CookbookVersion

	// basically we have to iterate each file in the cookbook, adding them
	// into a tar as we get them. We then gzip that and send it along.
    sections := [][]struct {chef.CookbookItem}{cookbook.Files, cookbook.Templates, cookbook.Recipes, cookbook.Attributes, cookbook.Definitions, cookbook.Libraries, cookbook.Providers, cookbook.Resources, cookbook.RootFiles}

    for _, section := range sections {
        log.Printf("Section is %v", section)
    	for _, file := range section {
    		log.Printf("Cookbook file is %s - %s", file.Name, file.Path)
    	}
    }

	return buf, nil
}

//----------

func (h *UniverseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	json, err := h.chef.Universe()

	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), 500)
		return
	}

	w.Write(json)
}
