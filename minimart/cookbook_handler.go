package minimart

import (
	_ "archive/tar"
	"fmt"
	_ "io/ioutil"
	"log"
	"net/http"
	"strings"
)

type CookbookHandler struct {
	chef *Minimart
}

func (h *CookbookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// ghetto matcher for the MVP!
	parts := strings.Split(r.URL.Path, "/")
	log.Printf("parts is %v", parts)
	buf, err := h.chef.CreateCookbookVersionTarball(parts[2], parts[3])

	if err != nil {
		http.Error(w, fmt.Sprintf("%v", err), 500)
		return
	}

	// FIXME: I'm sure we need some headers, etc
	r.Write(buf)
}
