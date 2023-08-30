// Refernce(s): https://www.hoodoo.digital/blog/how-to-make-a-web-crawler-using-go-and-colly-tutorial
// https://code.visualstudio.com/docs/cpp/config-mingw
// https://developer.fyne.io/
package main

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/gocolly/colly/v2"

	"net/http"
	"time"
)

// CustomError is a struct to hold error messages and additional information.
type CustomError struct {
	Message string
	Code    int
}

// Implement the error interface for CustomError.
func (e CustomError) Error() string {
	return fmt.Sprintf("Error: %s (Code: %d)", e.Message, e.Code)
}

// fetchURL measures the time taken to fetch the content of a URL.
func fetchURL(urlStr string) (time.Duration, error) {
	start := time.Now()

	resp, err := http.Get(urlStr)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("got HTTP status code %d", resp.StatusCode)
	}

	return time.Since(start), nil
}


// Check URL structure and SEO-friendliness
func checkURLValidity(inputURL string) []error {
	var errors []error
	u, err := url.Parse(inputURL)

	if err != nil {
		// Return a CustomError instance for the parsing error
		errors = append(errors, CustomError{
			Message: "There was a parsing error",
			Code:    500,
		})
	}

	// Check for a valid scheme (http or https)
	if u.Scheme != "http" && u.Scheme != "https" {
		// Return a CustomError instance for the HTTP error
		errors = append(errors, CustomError{
			Message: "HTTP Error",
			Code:    400,
		})
	}

	// Check for SEO-friendliness of the URL
	// Define the maximum allowed URL length
	maxURLLength := 100

	// Compare the length of the URL's path with the maximum allowed length
	if len(u.Path) > maxURLLength {
		errors = append(errors, CustomError{
			Message: "SEO-Friendliness violation length of URL's path is over 100",
			Code:    199,
		})
	}

	// Check if the URL path is descriptive and readable (no query strings or fragments)
	if u.RawQuery != "" || u.Fragment != "" {
		errors = append(errors, CustomError{
			Message: "SEO-Friendliness violation URL's path of either the query string or fragment is empty",
			Code:    199,
		})
	}

	// Check if the URL path contains only lowercase letters and hyphens (SEO-friendly characters)
	if u.Path != strings.ToLower(u.Path) || strings.Contains(u.Path, "_") {
		errors = append(errors, CustomError{
			Message: "SEO-Friendliness Violation URL's path contains uppercase letters and/or hyphens",
			Code:    199,
		})
	}

	// Return all errors
	return errors
}

// Define a map to track visited domains and their corresponding URLs
var visitedDomains = make(map[string]bool)
var visitedDomainsMutex sync.Mutex

func normalizeURL(inputURL string) (string, error) {
	u, err := url.Parse(inputURL)
	if err != nil {
		return "", err
	}

	// Normalize the URL by removing www. and trailing slashes
	host := strings.TrimPrefix(u.Hostname(), "www.")
	return fmt.Sprintf("%s://%s", u.Scheme, host), nil
}
func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Go Web Crawler")
	myWindow.Resize(fyne.NewSize(800, 450))

	entry := widget.NewEntry()
	entry.SetPlaceHolder("https://www.example.com")

	crawlingLogsLabel := widget.NewLabel("Crawling Logs")
	crawlingLogsLabel.Alignment = fyne.TextAlignLeading

	crawlingLogs := widget.NewLabel("")

	scrollContainer := container.NewVScroll(crawlingLogs)
	scrollContainer.SetMinSize(fyne.NewSize(800, 500))

	startButton := widget.NewButtonWithIcon("Start Crawling", theme.ComputerIcon(), func() {
		inputURLs := strings.Fields(entry.Text)
		if len(inputURLs) > 0 {
			crawlingLogs.SetText("") // Clear the crawling logs before starting the crawling process.

			concurrentRequests := make(chan struct{}, 10)
			var wg sync.WaitGroup

			c := colly.NewCollector(
				colly.MaxDepth(0),
				colly.Async(true),
				colly.IgnoreRobotsTxt(),
			)

			c.OnScraped(func(r *colly.Response) {
				crawlingLogs.SetText(crawlingLogs.Text + fmt.Sprintf("Finished %s\n", r.Request.URL))
				wg.Done()
			})

			c.OnError(func(r *colly.Response, err error) {
				crawlingLogs.SetText(crawlingLogs.Text + fmt.Sprintf("Something went wrong, https:%s\n\n", err))
				wg.Done()
			})

			normalizedURLs := make(map[string]bool)
			normalizedURLsMutex := sync.Mutex{}

			duplicateLogs := map[string][]string{} // Group duplicate/similar URLs
			
			newLine := widget.NewLabel("\n")
			for _, inputURL := range inputURLs {
				wg.Add(1)
				concurrentRequests <- struct{}{}
				
				// Handle the list of errors returned by checkURLValidity()
				errors := checkURLValidity(inputURL)
				if len(errors) > 0 {
					crawlingLogs.SetText(crawlingLogs.Text + fmt.Sprintf("url: %s\n", inputURL))
					for _, err := range errors {
						crawlingLogs.SetText(crawlingLogs.Text + err.Error() + "\n")
					}
				}
				newLine.SetText(newLine.Text)

				normalizedURL, err := normalizeURL(inputURL)
				if err != nil {
					crawlingLogs.SetText(crawlingLogs.Text + fmt.Sprintf("Invalid URL: %s\n", inputURL))
					wg.Done()
					<-concurrentRequests
					continue
				}

				normalizedURLsMutex.Lock()
				if normalizedURLs[normalizedURL] {
					duplicateLogs[normalizedURL] = append(duplicateLogs[normalizedURL], inputURL)
					wg.Done()
					normalizedURLsMutex.Unlock()
					<-concurrentRequests
					continue
				}
				normalizedURLs[normalizedURL] = true
				normalizedURLsMutex.Unlock()

				// Also add the URL with the opposite scheme (http vs. https)
				oppositeSchemeURL := toggleScheme(normalizedURL)
				normalizedURLsMutex.Lock()
				normalizedURLs[oppositeSchemeURL] = true
				normalizedURLsMutex.Unlock()

				go func(url string) {
					// Measure loading speed
					duration, err := fetchURL(url)
					if err != nil {
						crawlingLogs.SetText(crawlingLogs.Text + fmt.Sprintf("Failed to fetch %s: %s\n", url, err))
						<-concurrentRequests
						wg.Done()
						return
					}
					crawlingLogs.SetText(crawlingLogs.Text + fmt.Sprintf("Loaded %s in %v\n", url, duration))

					// Now let Colly visit and scrape the site
					c.Visit(url)
					<-concurrentRequests
				}(inputURL)

			}

			for _, similarURLs := range duplicateLogs {
				crawlingLogs.SetText(crawlingLogs.Text + fmt.Sprintf("Duplicate or Similar URLs:\n- %s\n", strings.Join(similarURLs, "\n- ")))
			}

			wg.Wait()
		} else {
			crawlingLogs.SetText("Please enter at least one valid URL.")
		}
	})

	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Enter website urls (separate by spaces and limit to 25):", Widget: entry},
		},
	}

	clearBtn := widget.NewButtonWithIcon("Reset", theme.CancelIcon(), func() {
		crawlingLogs.SetText("")
		entry.SetText("")
	})

	myWindow.SetContent(container.NewVBox(form, crawlingLogsLabel, scrollContainer, clearBtn, startButton))
	myWindow.ShowAndRun()
}

// Helper function to toggle between http and https schemes
func toggleScheme(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	if u.Scheme == "http" {
		u.Scheme = "https"
	} else if u.Scheme == "https" {
		u.Scheme = "http"
	}
	return u.String()
}
