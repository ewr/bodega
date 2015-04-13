package minimart

import (
	_ "archive/tar"
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	_ "errors"
	"fmt"
	_ "github.com/ctdk/goiardi/cookbook"
	_ "github.com/go-chef/chef"
	"github.com/marpaia/chef-golang"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
    "strings"
)

type ProxyCookbookVersion struct {
	LocationPath    string                `json:"location_path"`
	LocationType    string                `json:"location_type"`
	Dependencies    map[string]string     `json:"dependencies"`
	CookbookVersion *chef.CookbookVersion `json:"-"`
}

type UniverseHandler struct {
	chef *Minimart
}

type CookbookHandler struct {
	chef *Minimart
}

type Minimart struct {
    Config *Config

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
		Url:         config.ChefServer,
		Key:         rsaKey,
		UserId:      config.ChefClient,
		SSLNoVerify: config.SkipSSL,
		Version:     "11.6.0",
	}

	return &Minimart{
        Config:     config,
        cookbooks:  make(map[string]map[string]*ProxyCookbookVersion),
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

func (c *Minimart) CreateCookbookVersionTarball(name, version string) (*bytes.Reader, error) {
	// Make sure we know this version
	if c.cookbooks[name] == nil || c.cookbooks[name][version] == nil {
		return nil, fmt.Errorf("Cookbook version not found: %s/%s", name, version)
	}

    fetch := c.NewCookbookTarballFetch(name,version,c.cookbooks[name][version].CookbookVersion,c.Config.SkipSSL)

    return fetch.Run()
}

//----------

func (h *UniverseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	json, err := h.chef.Universe()

	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), 500)
		return
	}

    w.Header().Set("Content-type","application/json")

	w.Write(json)
}

//----------

func (h *CookbookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// ghetto matcher for the MVP!
	parts := strings.Split(r.URL.Path, "/")
	log.Printf("parts is %v", parts)
	buf, err := h.chef.CreateCookbookVersionTarball(parts[2], parts[3])
    log.Printf("buf len is %v", buf.Len())

	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), 500)
		return
	}

    w.Header().Set("Content-type","application/octet-stream")

	// FIXME: I'm sure we need some headers, etc
	buf.WriteTo(w)
}