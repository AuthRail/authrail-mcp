package healthprobe

import (
	"errors"
	"net/http"
	"net/url"
	"time"
)

func Run(address string) error {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.User != nil {
		return errors.New("invalid health address")
	}
	client := http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(address)
	if err != nil {
		return errors.New("service not ready")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("service not ready")
	}
	return nil
}
