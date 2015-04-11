package main

import (
	_ "archive/tar"
	"flag"
	"github.com/ewr/minimart/minimart"
	"log"
	"net/http"
	"time"
)

//----------

func main() {
	var (
		chefServer   = flag.String("chef.server", "", "Chef server URL (with org, if applicable)")
		chefPEM      = flag.String("chef.pem", "", "Path to Chef user PEM file")
		chefClient   = flag.String("chef.client", "", "Client to use when connecting to Chef Server")
		port         = flag.String("listen", ":8080", "Listening address")
		baseURL      = flag.String("baseURL", "http://localhost:8080", "Base URL for universe")
		pollInterval = flag.Duration("chef.interval", 5*time.Minute, "Interval in seconds to poll for cookbook updates.")
		skipSSL      = flag.Bool("skip-ssl", true, "Turn off Chef Server SSL verification")
	)
	flag.Parse()

	mart := minimart.NewMinimart(&minimart.Config{
		ChefServer: *chefServer,
		ChefPEM:    *chefPEM,
		ChefClient: *chefClient,
		BaseURL:    *baseURL,
		SkipSSL:    *skipSSL,
	})

	log.Printf("Chef server connected!")

	go mart.PollForCookbooks(*pollInterval)

	http.Handle("/cookbooks/", mart.NewCookbookHandler())
	http.Handle("/universe", mart.NewUniverseHandler())

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Minimart</title></head>
             <body>
             <h1>Minimart</h1>
             </body>
             </html>`))
	})

	log.Fatal(http.ListenAndServe(*port, nil))
}
