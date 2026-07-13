// Package api contains all the api used by XrayR
// To implement an api, one needs to implement the API interface.
package api

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/go-resty/resty/v2"
)

// BaseClient holds fields and methods common to every panel
// implementation. Embed it in your panel's APIClient to inherit
// the shared assembleURL helper. The shared HTTP error checks
// are exposed as CheckResponse so each panel can compose its
// own parseResponse that knows which JSON shape the panel
// returns.
type BaseClient struct {
	// APIHost is the base URL of the panel's API. assembleURL
	// concatenates it with a per-call path.
	APIHost string
}

// AssembleURL joins the panel base URL with the per-request
// path. It is exposed both as a method on BaseClient and as a
// free function so callers that don't embed BaseClient can
// still use it.
func (b *BaseClient) AssembleURL(path string) string {
	return b.APIHost + path
}

// AssembleURLFunc is the free-function form used by panel
// packages that prefer to keep their existing signature.
func AssembleURLFunc(host, path string) string {
	return host + path
}

// CheckResponse runs the common error and 4xx checks that
// every panel's parseResponse used to duplicate. It returns
// nil if the request was transport-successful and the panel
// returned an HTTP status below 400, otherwise an error
// describing the failure. The caller is responsible for the
// per-panel JSON shape validation (different panels return
// different status fields like ret, StatusCode, Status, etc.).
func CheckResponse(res *resty.Response, path string, err error) error {
	if err != nil {
		return fmt.Errorf("request %s failed: %w", path, err)
	}
	if res.StatusCode() > 400 {
		return fmt.Errorf("request %s failed: %s", path, string(res.Body()))
	}
	return nil
}

// ReadLocalRuleList reads the local rule list file
func ReadLocalRuleList(path string) (LocalRuleList []DetectRule) {
	LocalRuleList = make([]DetectRule, 0)
	if path != "" {
		// open the file
		file, err := os.Open(path)
		defer func() {
			if file != nil {
				_ = file.Close()
			}
		}()
		// handle errors while opening
		if err != nil {
			fmt.Printf("Error when opening file: %s\n", err)
			return LocalRuleList
		}

		fileScanner := bufio.NewScanner(file)

		// read line by line
		for fileScanner.Scan() {
			LocalRuleList = append(LocalRuleList, DetectRule{
				ID:      -1,
				Pattern: regexp.MustCompile(fileScanner.Text()),
			})
		}
		// handle first encountered error while reading
		if err := fileScanner.Err(); err != nil {
			fmt.Printf("Error while reading rule list %s: %s\n", path, err)
			return
		}
	}
	return LocalRuleList
}

// CreateRestyClient creates a resty client with common configurations
// including connection pool and keep-alive settings.
func CreateRestyClient(apiConfig *Config) *resty.Client {
	client := resty.New()
	client.SetRetryCount(3)
	if apiConfig.Timeout > 0 {
		client.SetTimeout(time.Duration(apiConfig.Timeout) * time.Second)
	} else {
		client.SetTimeout(5 * time.Second)
	}

	// Configure connection pool and keep-alive
	if transport, ok := client.GetClient().Transport.(*http.Transport); ok {
		transport.MaxIdleConns = 100
		transport.MaxIdleConnsPerHost = 20
		transport.MaxConnsPerHost = 100
		transport.IdleConnTimeout = 90 * time.Second
	}

	client.SetBaseURL(apiConfig.APIHost)
	return client
}
