// Package main implements a simple, single-threaded PDF scraper and downloader. // Declare this file as part of the main package so it can be compiled into an executable.
package main // Start the main package declaration.

// Import all the standard library packages this program needs. // Comment explaining the purpose of the import block below.
import ( // Begin the import block.
	"fmt"           // Import "fmt" so we can build formatted error strings with fmt.Errorf.
	"io"            // Import "io" so we can copy bytes from an HTTP response body into a local file.
	"log"           // Import "log" so we can print timestamped status and error messages instead of plain fmt.Println calls.
	"net/http"      // Import "net/http" so we can make HTTP GET requests to fetch the web page and the PDF files.
	"os"            // Import "os" so we can create folders, create/rename files, and check if files already exist.
	"path/filepath" // Import "path/filepath" so we can build local file paths and extract file names in a platform-correct way.
	"regexp"        // Import "regexp" so we can find every ".pdf" link inside the raw HTML using a regular expression.
	"strings"       // Import "strings" so we can perform basic string checks and cleanups.
	"time"          // Import "time" so we can configure sensible timeouts for our HTTP requests.
) // Close the import block.

// targetPageURL holds the web page we want to visit and scrape for PDF links. // Comment explaining the constant below.
const targetPageURL = "https://awqinc.com/water-treatment-equipment-manuals/" // Define the URL of the page containing the manuals.

// outputFolderName holds the name of the local folder where PDFs will be saved. // Comment explaining the constant below.
const outputFolderName = "PDFs" // Define the folder name exactly as requested, "PDFs/".

// httpRequestTimeout caps how long any single HTTP request is allowed to take before it is aborted. // Comment explaining the constant below.
const httpRequestTimeout = 60 * time.Second // Define a generous but finite timeout so a stalled server cannot hang the program forever.

// httpUserAgent is sent with every request so the remote server sees a normal, identifiable client. // Comment explaining the constant below.
const httpUserAgent = "Mozilla/5.0 (compatible; SimplePDFDownloader/1.0)" // Define a realistic User-Agent string to avoid being blocked by basic bot filters.

// sharedHTTPClient is the single http.Client reused for every request the program makes. // Comment explaining the variable below.
var sharedHTTPClient = &http.Client{Timeout: httpRequestTimeout} // Create one client with our timeout so every call benefits from it, instead of using the default client.

// main is the entry point of the program and drives the whole scrape-and-download process. // Comment explaining the function below.
func main() { // Begin the main function.
	log.SetFlags(log.Ldate | log.Ltime)       // Configure the logger to prefix every line with the date and time, so progress can be tracked.
	log.Println("Starting PDF downloader...") // Log a start message so the user knows the program has begun.

	htmlContent, fetchErr := fetchPageHTML(targetPageURL) // Fetch the raw HTML of the target page and capture any error.
	if fetchErr != nil {                                  // Check if fetching the page failed.
		log.Fatalf("Error fetching page %q: %v", targetPageURL, fetchErr) // Log the fatal error and exit immediately, since we cannot continue without the page HTML.
	} // Close the if block for the fetch error check.

	pdfLinks := extractPDFLinks(htmlContent)                       // Extract every PDF link found inside the fetched HTML content.
	log.Printf("Found %d PDF link(s) on the page.", len(pdfLinks)) // Log how many PDF links were discovered.

	if mkdirErr := os.MkdirAll(outputFolderName, 0755); mkdirErr != nil { // Create the output folder (and any missing parents); check the returned error inline.
		log.Fatalf("Error creating output folder %q: %v", outputFolderName, mkdirErr) // Log the fatal error and exit, since we have nowhere to save files.
	} // Close the if block for the folder creation error check.

	var downloadedCount, skippedCount, failedCount int // Declare three counters to summarize the run: files downloaded, files skipped, and files that failed.

	for _, pdfURL := range pdfLinks { // Loop over every PDF link one at a time (no concurrency, as requested).
		wasSkipped, downloadErr := downloadPDFIfMissing(pdfURL) // Attempt to download the current PDF; learn whether it was skipped and whether an error occurred.
		switch {                                                // Begin a switch statement to update the correct counter based on the outcome.
		case downloadErr != nil: // Check if the download attempt returned an error.
			log.Printf("Error downloading %q: %v", pdfURL, downloadErr) // Log which URL failed and why, but keep going with the rest.
			failedCount++                                               // Increment the failure counter.
		case wasSkipped: // Check if the file was skipped because it already existed.
			skippedCount++ // Increment the skipped counter.
		default: // Handle the remaining case, which is a successful new download.
			downloadedCount++ // Increment the downloaded counter.
		} // Close the switch statement.
	} // Close the loop over all PDF links.

	log.Printf("All done. Downloaded: %d, Skipped: %d, Failed: %d.", downloadedCount, skippedCount, failedCount) // Log a final summary of the whole run.
} // Close the main function.

// fetchPageHTML performs an HTTP GET request against the given URL and returns the response body as a string. // Comment explaining the function below.
func fetchPageHTML(pageURL string) (string, error) { // Begin the fetchPageHTML function, taking a URL and returning the HTML string plus an error.
	httpRequest, buildErr := http.NewRequest(http.MethodGet, pageURL, nil) // Build the GET request explicitly so we can attach a custom header.
	if buildErr != nil {                                                   // Check if constructing the request object itself failed (e.g., malformed URL).
		return "", fmt.Errorf("building request for %q: %w", pageURL, buildErr) // Wrap and return the build error with context about which URL caused it.
	} // Close the if block for the request-building error check.
	httpRequest.Header.Set("User-Agent", httpUserAgent) // Attach our User-Agent header to look like a normal browser request.

	httpResponse, httpErr := sharedHTTPClient.Do(httpRequest) // Send the GET request using our shared, timeout-bound HTTP client.
	if httpErr != nil {                                       // Check if the HTTP request itself failed (e.g., network problem or timeout).
		return "", fmt.Errorf("performing request for %q: %w", pageURL, httpErr) // Wrap and return the request error with context.
	} // Close the if block for the HTTP request error check.
	defer httpResponse.Body.Close() // Ensure the response body is closed once this function returns, to avoid leaking resources.

	if httpResponse.StatusCode != http.StatusOK { // Check if the server responded with anything other than 200 OK.
		return "", fmt.Errorf("unexpected status %s for %q", httpResponse.Status, pageURL) // Return an error describing the unexpected HTTP status.
	} // Close the if block for the status code check.

	bodyBytes, readErr := io.ReadAll(httpResponse.Body) // Read the entire response body into a byte slice.
	if readErr != nil {                                 // Check if reading the body failed partway through.
		return "", fmt.Errorf("reading response body for %q: %w", pageURL, readErr) // Wrap and return the read error with context.
	} // Close the if block for the read error check.

	return string(bodyBytes), nil // Convert the byte slice to a string and return it with no error.
} // Close the fetchPageHTML function.

// extractPDFLinks scans raw HTML text and returns a slice of every absolute URL ending in ".pdf". // Comment explaining the function below.
func extractPDFLinks(htmlContent string) []string { // Begin the extractPDFLinks function, taking HTML text and returning a slice of URL strings.
	pdfLinkPattern := regexp.MustCompile(`https?://[^\s"'()<>]+\.pdf`) // Compile a regular expression that matches http/https URLs ending in ".pdf".
	rawMatches := pdfLinkPattern.FindAllString(htmlContent, -1)        // Find every substring in the HTML that matches the PDF URL pattern.

	uniqueLinksSeen := make(map[string]bool) // Create a map to track which URLs we have already added, so we avoid duplicates.
	var uniquePDFLinks []string              // Declare a slice that will hold the final deduplicated list of PDF URLs.

	for _, rawURL := range rawMatches { // Loop over every raw matched URL found by the regular expression.
		cleanedURL := strings.TrimSpace(rawURL) // Trim any accidental leading or trailing whitespace from the matched URL.
		if !uniqueLinksSeen[cleanedURL] {       // Check if we have not already recorded this exact URL.
			uniqueLinksSeen[cleanedURL] = true                  // Mark this URL as seen so we never add it twice.
			uniquePDFLinks = append(uniquePDFLinks, cleanedURL) // Append the cleaned, unique URL to our result slice.
		} // Close the if block for the duplicate check.
	} // Close the loop over raw matches.

	return uniquePDFLinks // Return the final slice of unique PDF URLs.
} // Close the extractPDFLinks function.

// downloadPDFIfMissing checks whether a PDF has already been downloaded, and if not, downloads it into the // Comment explaining the function below (line 1 of 2).
// output folder. It returns whether the file was skipped (already present) and any error that occurred. // Comment explaining the function below (line 2 of 2).
func downloadPDFIfMissing(pdfURL string) (bool, error) { // Begin the downloadPDFIfMissing function, returning a "was skipped" flag plus an error.
	fileName := filepath.Base(pdfURL) // Extract just the file name (last path segment) from the full PDF URL.

	finalFilePath := filepath.Join(outputFolderName, fileName) // Build the final local file path by joining the output folder and the file name safely.

	if _, statErr := os.Stat(finalFilePath); statErr == nil { // Ask the operating system if the final file already exists; check the returned error inline.
		log.Printf("Skipping (already exists): %s", fileName) // Log a message telling the user we are skipping this already-downloaded file.
		return true, nil                                      // Report that the file was skipped, with no error.
	} // Close the if block for the "file already exists" check.

	log.Printf("Downloading: %s", fileName) // Log a message telling the user we are about to download this new file.

	httpRequest, buildErr := http.NewRequest(http.MethodGet, pdfURL, nil) // Build the GET request explicitly so we can attach a custom header.
	if buildErr != nil {                                                  // Check if constructing the request object itself failed.
		return false, fmt.Errorf("building request for %q: %w", pdfURL, buildErr) // Wrap and return the build error with context.
	} // Close the if block for the request-building error check.
	httpRequest.Header.Set("User-Agent", httpUserAgent) // Attach our User-Agent header so the server treats this like a normal request.

	httpResponse, httpErr := sharedHTTPClient.Do(httpRequest) // Send the GET request using our shared, timeout-bound HTTP client.
	if httpErr != nil {                                       // Check if the HTTP request for the PDF failed.
		return false, fmt.Errorf("performing request for %q: %w", pdfURL, httpErr) // Wrap and return the request error with context.
	} // Close the if block for the HTTP request error check.
	defer httpResponse.Body.Close() // Ensure the PDF response body is closed once this function returns.

	if httpResponse.StatusCode != http.StatusOK { // Check if the server responded with anything other than 200 OK.
		return false, fmt.Errorf("unexpected status %s for %q", httpResponse.Status, pdfURL) // Return an error describing the unexpected HTTP status.
	} // Close the if block for the status code check.

	temporaryFilePath := finalFilePath + ".downloading" // Build a temporary file path used while the download is still in progress.

	temporaryFile, createErr := os.Create(temporaryFilePath) // Create (or truncate) the temporary file that will hold the downloaded bytes.
	if createErr != nil {                                    // Check if creating the temporary file failed.
		return false, fmt.Errorf("creating temp file %q: %w", temporaryFilePath, createErr) // Wrap and return the file creation error with context.
	} // Close the if block for the file creation error check.

	_, copyErr := io.Copy(temporaryFile, httpResponse.Body) // Stream (copy) all bytes from the HTTP response body directly into the temporary file.
	closeErr := temporaryFile.Close()                       // Close the temporary file right away so all buffered data is flushed to disk before we rename it.

	if copyErr != nil { // Check if copying the bytes failed partway through.
		os.Remove(temporaryFilePath)                                         // Remove the incomplete temporary file so it cannot be mistaken for a finished download.
		return false, fmt.Errorf("copying data for %q: %w", pdfURL, copyErr) // Wrap and return the copy error with context.
	} // Close the if block for the copy error check.
	if closeErr != nil { // Check if closing (and thus flushing) the temporary file failed.
		os.Remove(temporaryFilePath)                                                      // Remove the possibly-corrupt temporary file to avoid leaving bad data behind.
		return false, fmt.Errorf("closing temp file %q: %w", temporaryFilePath, closeErr) // Wrap and return the close error with context.
	} // Close the if block for the close error check.

	if renameErr := os.Rename(temporaryFilePath, finalFilePath); renameErr != nil { // Atomically rename the completed temp file to its final name; check the error inline.
		return false, fmt.Errorf("renaming %q to %q: %w", temporaryFilePath, finalFilePath, renameErr) // Wrap and return the rename error with context.
	} // Close the if block for the rename error check.

	log.Printf("Saved: %s", finalFilePath) // Log a confirmation message showing where the file was saved.
	return false, nil                      // Report that the file was not skipped (it was freshly downloaded) and that there was no error.
} // Close the downloadPDFIfMissing function.
