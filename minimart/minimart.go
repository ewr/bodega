package minimart

import (
	_ "archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	_ "github.com/ctdk/goiardi/cookbook"
	"github.com/go-chef/chef"
	_ "github.com/marpaia/chef-golang"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type ProxyCookbookVersion struct {
	LocationPath string            `json:"location_path"`
	LocationType string            `json:"location_type"`
	Dependencies map[string]string `json:"dependencies"`
	Cookbook     chef.Cookbook     `json:"-"`
}

type UniverseHandler struct {
	chef *Minimart
}

type Minimart struct {
	baseURL string

	cookbooks  map[string]map[string]*ProxyCookbookVersion
	chefServer string
	client     *chef.Client
	ticker     *time.Ticker
	tickerDone chan struct{}

	pollMutex sync.RWMutex
}

type Config struct {
    ChefServer string
    ChefPEM string
    ChefClient string
    BaseURL string
    SkipSSL bool
}

func NewMinimart(config *Config) *Minimart {
	// read PEM file
	key, err := ioutil.ReadFile(config.ChefPEM)
	if err != nil {
		fmt.Println("Couldn't read key.pem:", err)
		os.Exit(1)
	}

	client, err := chef.NewClient(&chef.Config{
		Name:    config.ChefClient,
		Key:     string(key),
		BaseURL: config.ChefServer,
		SkipSSL: true,
	})

	if err != nil {
		log.Fatal("Error connecting to Chef server: %v", err)
	}

	//client.SSLNoVerify = true

	return &Minimart{
		baseURL:    config.BaseURL,
		chefServer: config.ChefServer,
		client:     client,
		ticker:     nil,
		tickerDone: make(chan struct{}),
	}
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

	all_versions, err := c.client.Cookbooks.ListAvailableVersions("all")

	if err != nil {
		log.Printf("Error fetching cookbooks: %v", err)
		return
	}

	var cookbooks = make(map[string]map[string]*ProxyCookbookVersion)

	for name, cvlist := range all_versions {
		log.Printf("got cookbook %v", name)

		versions := make(map[string]*ProxyCookbookVersion)
		for _, v := range cvlist.Versions {
			cv, err := c.client.Cookbooks.GetVersion(name, v.Version)

			if err != nil {
				log.Printf("error fetching cookbook version: %v", err)
				break
			}

			versions[v.Version] = &ProxyCookbookVersion{
				LocationPath: fmt.Sprintf("%s/cookbooks/%s/%s/download", c.baseURL, name, v.Version),
				LocationType: "supermarket",
				Dependencies: cv.Metadata.Depends,
				Cookbook:     cv,
			}

			log.Printf("got %v/%v", name, v.Version)
		}

		cookbooks[name] = versions
	}

	c.cookbooks = cookbooks
	log.Printf("Cookbook cache complete.")
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

	cookbook := c.cookbooks[name][version].Cookbook

	// basically we have to iterate each file in the cookbook, adding them
	// into a tar as we get them. We then gzip that and send it along.
	for _, file := range cookbook.Files {
		log.Printf("Cookbook file is %s", file.Name)
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
