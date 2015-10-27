package bodega

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/marpaia/chef-golang"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

func (c *Bodega) PollForCookbooks(interval time.Duration) {
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
			c.scrapeCookbooks()
		case <-c.tickerDone:
			log.Printf("Should stop polling.")
			c.ticker.Stop()
			return
		}
	}
}

//----------

func (c *Bodega) StopPollingForCookbooks() {
	c.tickerDone <- struct{}{}
}

//----------

func (c *Bodega) scrapeCookbooks() {
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

	for name, cvlist := range all_versions {
		log.Printf("got cookbook %v", name)

		if c.cookbooks[name] == nil {
			c.cookbooks[name] = make(map[string]*ProxyCookbookVersion)
		}

		for _, v := range cvlist.Versions {
			if c.cookbooks[name][v.Version] != nil {
				log.Printf("Skipping known cookbook version %s/%s", name, v.Version)
				continue
			}

			cv, ok, err := c.client.GetCookbookVersion(name, v.Version)

			if err != nil {
				log.Printf("error fetching cookbook version: %v", err)
				break
			}

			if !ok {
				log.Printf("Couldn't find cookbook version: %s/%s", name, v.Version)
				break
			}

			c.cookbooks[name][v.Version] = &ProxyCookbookVersion{
				DownloadURL:     fmt.Sprintf("%s/cookbooks/%s/%s/download", c.baseURL, name, v.Version),
				LocationPath:    fmt.Sprintf("%s/cookbooks/%s/%s/download", c.baseURL, name, v.Version),
				LocationType:    "uri",
				Dependencies:    cv.Metadata.Dependencies,
				CookbookVersion: cv,
			}

			log.Printf("got %v/%v", name, v.Version)
		}
	}

	log.Printf("Cookbook cache run complete.")
}

//----------

func (c *Bodega) getAllCookbooks() (map[string]*chef.Cookbook, error) {
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
