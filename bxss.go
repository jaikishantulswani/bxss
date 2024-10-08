package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"golang.org/x/time/rate"
)

const (
	BannerColor  = "\033[1;34m%s\033[0m\033[1;36m%s\033[0m"
	TextColor    = "\033[1;0m%s\033[1;32m%s\n\033[0m"
	InfoColor    = "\033[1;0m%s\033[1;35m%s\033[0m"
	NoticeColor  = "\033[1;0m%s\033[1;34m%s\n\033[0m"
	WarningColor = "\033[1;33m%s%s\033[0m"
	ErrorColor   = "\033[1;31m%s%s\033[0m"
	DebugColor   = "\033[0;36m%s%s\033[0m"
)

var (
	debug bool
	showTimestamp bool
	limiter *rate.Limiter
)

func main() {
	// Flag variables
	var c int
	var p string
	var pf string
	var h string
	var hf string
	var a bool
	var t bool
	var rl float64
	var r bool

	// The flag / arguments
	flag.IntVar(&c, "concurrency", 30, "Set the concurrency")
	flag.StringVar(&h, "header", "", "Set a single custom header")
	flag.StringVar(&hf, "headerFile", "", "Path to file containing headers to test")
	flag.StringVar(&p, "payload", "", "The blind XSS payload")
	flag.StringVar(&pf, "payloadFile", "", "Path to file containing payloads to test")
	flag.BoolVar(&a, "appendMode", false, "Append the payload to the parameter")
	flag.BoolVar(&t, "parameters", false, "Test the parameters for blind xss")
	flag.BoolVar(&debug, "debug", false, "Enable debug mode to view full request details")
	flag.Float64Var(&rl, "rl", 0, "Rate limit in requests per second (optional)")
	flag.BoolVar(&r, "r", false, "Follow redirects (optional)")
	flag.BoolVar(&showTimestamp, "ts", false, "Show timestamp for each request (optional)")

	// Parse the arguments
	flag.Parse()

	// The banner
	fmt.Printf(BannerColor, `
	  ____               
	 |  _ \              
 	 | |_) |_  _____ ___ 
	 |  * <\ \/ / *_/ __|
	 | |_) |>  <\__ \__ \
	 |____//_/\_\___/___/
	                     
                    
	`, "-- Coded by @z0idsec -- \n")

	// Check if at least one header and one payload option is provided
	if (h == "" && hf == "") || (p == "" && pf == "") {
		flag.PrintDefaults()
		return
	}

	if rl > 0 {
		limiter = rate.NewLimiter(rate.Limit(rl), 1)
	}

	var headers []string
	if hf != "" {
		var err error
		headers, err = readLinesFromFile(hf)
		if err != nil {
			fmt.Printf(ErrorColor, "Error reading header file: ", err.Error())
			return
		}
	} else if h != "" {
		headers = []string{h}
	}

	var payloads []string
	if pf != "" {
		var err error
		payloads, err = readLinesFromFile(pf)
		if err != nil {
			fmt.Printf(ErrorColor, "Error reading payload file: ", err.Error())
			return
		}
	} else if p != "" {
		payloads = []string{p}
	}

	fmt.Printf(NoticeColor, "\n[-] Please Be Patient for bxss\n ", "")

	var wg sync.WaitGroup
	for i := 0; i < c; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processPayloadsAndHeaders(payloads, headers, a, t, r)
		}()
	}
	wg.Wait()
}

func readLinesFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, strings.TrimSpace(scanner.Text()))
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func processPayloadsAndHeaders(payloads, headers []string, appendMode, isParameters, followRedirects bool) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // Increase buffer size
	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !followRedirects {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	for scanner.Scan() {
		link := strings.TrimSpace(scanner.Text())
		if link == "" {
			continue // Skip empty lines
		}
		link = ensureProtocol(link)
		for _, payload := range payloads {
			for _, header := range headers {
				testbxss(client, payload, link, header, appendMode, isParameters)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf(ErrorColor, "Error reading input: ", err.Error())
	}
}

func ensureProtocol(link string) string {
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return "https://" + link
	}
	return link
}

func testbxss(client *http.Client, payload, link, header string, appendMode, isParameters bool) {
	if limiter != nil {
		limiter.Wait(context.Background())
	}
	time.Sleep(500 * time.Microsecond)
	fmt.Println("")
	fmt.Printf(NoticeColor, "[+] \tHeader:  ", header)
	fmt.Printf(TextColor, "[+] \tPayload: ", payload)
	fmt.Println("")

	methods := []string{"GET", "POST", "OPTIONS", "PUT"}
	for _, method := range methods {
		makeRequest(client, method, payload, link, header, appendMode, isParameters)
	}
}

func makeRequest(client *http.Client, method, payload, link, header string, appendMode, isParameters bool) {
	fmt.Printf(NoticeColor, "\n[*] Making request with ", method)
	fmt.Println("")
	
	u, err := url.Parse(link)
	if err != nil {
		fmt.Printf(ErrorColor, "Error parsing URL: ", err.Error())
		return
	}

	if isParameters {
		qs := u.Query()
		for param, vv := range qs {
			if appendMode {
				fmt.Printf(TextColor, "[*] Parameter:  ", param)
				qs.Set(param, vv[0]+payload)
			} else {
				fmt.Printf(TextColor, "[*] Parameter:  ", param)
				qs.Set(param, payload)
			}
		}
		u.RawQuery = qs.Encode()
	}

	fmt.Printf(InfoColor, "[-] Testing:  ", u.String())
	request, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		fmt.Printf(ErrorColor, "Error creating request: ", err.Error())
		return
	}

	// Remove existing headers that we're testing
	request.Header.Del("User-Agent")
	request.Header.Del("X-Forwarded-Host")
	request.Header.Del("X-Forwarded-For")

	// Set the header with the payload
	headerParts := strings.SplitN(header, ":", 2)
	if len(headerParts) == 2 {
		headerName := strings.TrimSpace(headerParts[0])
		headerValue := strings.TrimSpace(headerParts[1])
		
		// Special handling for User-Agent header
		if strings.ToLower(headerName) == "user-agent" {
			request.Header.Set("User-Agent", headerValue+payload)
		} else {
			request.Header.Set(headerName, headerValue+payload)
		}
	} else {
		// If no value is provided, use the payload as the value
		request.Header.Set(header, payload)
	}

	if showTimestamp {
		fmt.Printf(InfoColor, "\n[*] Timestamp: ", time.Now().Format(time.RFC3339))
	}

	if debug {
		debugRequest(request)
	}

	response, err := client.Do(request)
	if err != nil {
		fmt.Printf(ErrorColor, "Error making request: ", err.Error())
		return
	}
	defer response.Body.Close()

	if debug {
		debugResponse(response)
	}
}

func debugRequest(req *http.Request) {
	dump, err := httputil.DumpRequestOut(req, true)
	if err != nil {
		fmt.Printf(ErrorColor, "Error dumping request: ", err.Error())
	} else {
		fmt.Printf(DebugColor, "\n--- Request ---\n", string(dump))
	}
}

func debugResponse(resp *http.Response) {
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		fmt.Printf(ErrorColor, "Error dumping response: ", err.Error())
	} else {
		fmt.Printf(DebugColor, "\n--- Response ---\n", string(dump))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf(ErrorColor, "Error reading response body: ", err.Error())
	} else {
		fmt.Printf(DebugColor, "\n--- Response Body ---\n", string(body))
	}
}
