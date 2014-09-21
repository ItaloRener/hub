package github

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/github/hub/utils"
)

type verboseTransport struct {
	Transport   *http.Transport
	Verbose     bool
	OverrideURL *url.URL
}

func (t *verboseTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if t.Verbose {
		t.dumpRequest(req)
	}

	if t.OverrideURL != nil {
		req = cloneRequest(req)
		req.Header.Set("X-Original-Scheme", req.URL.Scheme)
		req.URL.Scheme = t.OverrideURL.Scheme
		req.URL.Host = t.OverrideURL.Host
	}

	resp, err = t.Transport.RoundTrip(req)

	if err == nil && t.Verbose {
		t.dumpResponse(resp)
	}

	return
}

func cloneRequest(req *http.Request) *http.Request {
	dup := new(http.Request)
	*dup = *req
	dup.URL, _ = url.Parse(req.URL.String())
	dup.Header = make(http.Header)
	for k, s := range req.Header {
		dup.Header[k] = s
	}
	return dup
}

func (t *verboseTransport) dumpRequest(req *http.Request) {
	info := fmt.Sprintf("> %s %s://%s%s", req.Method, req.URL.Scheme, req.Host, req.URL.Path)
	t.verbosePrintln(info)
	t.dumpHeaders(req.Header, ">")
	body := t.dumpBody(req.Body)
	if body != nil {
		// reset body since it's been read
		req.Body = body
	}
}

func (t *verboseTransport) dumpResponse(resp *http.Response) {
	info := fmt.Sprintf("< HTTP %d", resp.StatusCode)
	location, err := resp.Location()
	if err == nil {
		info = fmt.Sprintf("%s\n< Location: %s", info, location.String())
	}
	t.verbosePrintln(info)
	t.dumpHeaders(resp.Header, "<")
	body := t.dumpBody(resp.Body)
	if body != nil {
		// reset body since it's been read
		resp.Body = body
	}
}

func (t *verboseTransport) dumpHeaders(header http.Header, indent string) {
	dumpHeaders := []string{"Authorization", "X-GitHub-OTP", "Localtion"}
	for _, h := range dumpHeaders {
		v := header.Get(h)
		if v != "" {
			r := regexp.MustCompile("(?i)^(basic|token) (.+)")
			if r.MatchString(v) {
				v = r.ReplaceAllString(v, "$1 [REDACTED]")
			}

			info := fmt.Sprintf("%s %s: %s", indent, h, v)
			t.verbosePrintln(info)
		}
	}
}

func (t *verboseTransport) dumpBody(body io.ReadCloser) io.ReadCloser {
	if body == nil {
		return nil
	}

	defer body.Close()
	buf := new(bytes.Buffer)
	_, err := io.Copy(buf, body)
	utils.Check(err)

	if buf.Len() > 0 {
		t.verbosePrintln(buf.String())
	}

	return ioutil.NopCloser(buf)
}

func (t *verboseTransport) verbosePrintln(msg string) {
	if isTerminal(os.Stderr.Fd()) {
		msg = fmt.Sprintf("\\e[36m%s\\e[m", msg)
	}

	fmt.Fprintln(os.Stderr, msg)
}

func newHttpClient(testHost string, verbose bool) *http.Client {
	var testURL *url.URL
	if testHost != "" {
		testURL, _ = url.Parse(testHost)
	}
	tr := &verboseTransport{
		Transport:   &http.Transport{Proxy: proxyFromEnvironment},
		Verbose:     verbose,
		OverrideURL: testURL,
	}
	return &http.Client{Transport: tr}
}

// An implementation of http.ProxyFromEnvironment that isn't broken
func proxyFromEnvironment(req *http.Request) (*url.URL, error) {
	proxy := os.Getenv("http_proxy")
	if proxy == "" {
		proxy = os.Getenv("HTTP_PROXY")
	}
	if proxy == "" {
		return nil, nil
	}

	proxyURL, err := url.Parse(proxy)
	if err != nil || !strings.HasPrefix(proxyURL.Scheme, "http") {
		if proxyURL, err := url.Parse("http://" + proxy); err == nil {
			return proxyURL, nil
		}
	}

	if err != nil {
		return nil, fmt.Errorf("invalid proxy address %q: %v", proxy, err)
	}

	return proxyURL, nil
}
