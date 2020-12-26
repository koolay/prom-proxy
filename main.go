package main

import (
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
)

const (
	acceptHeader = `application/openmetrics-text; version=0.0.1,text/plain;version=0.0.4;q=0.5,*/*;q=0.1`
)

var (
	userAgentHeader       = fmt.Sprintf("Prometheus/%s", "2.7.1")
	scrapeTimeout         = 5 * time.Second
	scrapeLimit     int64 = 1024 * 1024 * 2 // bytes
)

type targetScraper struct {
	URL string
}
type ErrResponse struct {
	HTTPStatus string
	Code       int
	Text       string
	URL        string
}

func (e ErrResponse) Error() string {
	return fmt.Sprintf("Response Error: URL: %s\n http status: %s\n, text: %s\n", e.URL, e.HTTPStatus, e.Text)
}

func (s *targetScraper) scrape(ctx context.Context, wr io.Writer) error {
	var err error
	req, err := http.NewRequestWithContext(ctx, "GET", s.URL, nil)

	if err != nil {
		return err
	}

	req.Header.Add("Accept", acceptHeader)
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", fmt.Sprintf("%d", 10))

	client := &http.Client{}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != 301 && resp.StatusCode != 302 {
		text := ""

		if resp.Body != nil {
			responseBytes, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Print(err)
			}

			text = string(responseBytes)
		}

		return &ErrResponse{
			Code:       resp.StatusCode,
			HTTPStatus: resp.Status,
			Text:       text,
			URL:        s.URL,
		}
	}

	if resp.Header.Get("Content-Encoding") != "gzip" {
		_, err := io.Copy(wr, resp.Body)
		if err != nil {
			return err
		}
	} else {
		zipReader, gzerr := gzip.NewReader(resp.Body)
		if gzerr != nil {
			return gzerr
		}

		limitedReader := io.LimitReader(zipReader, scrapeLimit)
		_, err = io.Copy(wr, limitedReader)
	}

	return err
}

func handler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	metricsURL := r.RequestURI
	if metricsURL == "" {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "Not Found(Empty proxy argument)")

		return
	}

	metricsURL = strings.TrimRight(metricsURL, "/")
	metricsURL = strings.Replace(metricsURL, ":80", "", -1)
	metricsURL = strings.Replace(metricsURL, ":443", "", -1)

	tc := targetScraper{
		URL: metricsURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)

	defer cancel()

	err := tc.scrape(ctx, w)

	if err != nil {
		processFailure(w, r.RequestURI, err)
	}
}

func processFailure(w http.ResponseWriter, targetURL string, err error) {
	log.Printf("failed to scrape: %s, %+v", targetURL, err)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if e, ok := err.(*ErrResponse); ok {
		w.WriteHeader(e.Code)
		_, werr := io.WriteString(w, fmt.Sprintf("HTTP Reques Failed! \n%s", err.Error()))

		if werr != nil {
			log.Printf("%+v", werr)
		}

		return
	}

	_, werr := io.WriteString(w, fmt.Sprintf("HTTP Reques Failed! \n%s", err.Error()))

	if werr != nil {
		log.Printf("%+v", werr)
	}
}

func serveHTTP(addr string) error {
	router := httprouter.New()
	router.GET("/*subpath", handler)

	log.Println("Start listen:", addr)

	return http.ListenAndServe(addr, router)
}

func main() {
	var addr string

	flag.StringVar(&addr, "addr", ":8444", "http listen address")
	flag.Parse()

	go func() {
		if err := serveHTTP(addr); err != nil {
			panic(err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutdown Server ...")
}
