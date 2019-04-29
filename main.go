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
)

const acceptHeader = `application/openmetrics-text; version=0.0.1,text/plain;version=0.0.4;q=0.5,*/*;q=0.1`

var userAgentHeader = fmt.Sprintf("Prometheus/%s", "2.7.1")

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
	req, err := http.NewRequest("GET", s.URL, nil)
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
		zipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return err
		}
		_, err = io.Copy(wr, zipReader)
	}

	return err
}

func handler(w http.ResponseWriter, r *http.Request) {

	metricsURL := r.RequestURI
	if metricsURL == "" {
		w.WriteHeader(404)
		fmt.Fprintf(w, "Not Found(Empty proxy argument)")
		return
	}
	metricsURL = strings.Replace(metricsURL, ":80", "", -1)
	metricsURL = strings.Replace(metricsURL, ":443", "", -1)
	tc := targetScraper{
		URL: metricsURL,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err := tc.scrape(ctx, w)
	if err != nil {
		log.Print(err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if e, ok := err.(*ErrResponse); ok {
			w.WriteHeader(e.Code)
			io.WriteString(w, fmt.Sprintf("HTTP Reques Failed! \n%s", err.Error()))
		} else {
			// w.WriteHeader(500)
			io.WriteString(w, fmt.Sprintf("HTTP Reques Failed! \n%s", err.Error()))
		}
	}
}

func serveHTTP(addr string) error {

	http.HandleFunc("/metrics", handler)
	http.HandleFunc("/api/metrics", handler)
	http.HandleFunc("/proc/metrics", handler)

	go func() {
		fmt.Println("Start listen:", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutdown Server ...")
	return nil
}

func main() {

	var addr string
	flag.StringVar(&addr, "addr", ":8444", "http listen address")
	flag.Parse()
	serveHTTP(addr)
}
