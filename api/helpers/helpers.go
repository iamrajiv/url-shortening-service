package helpers

import (
	"os"
	"strings"
)

// EnforceHTTP is a function that enforces the use of "http://" prefix in the URL string, if it is not already present
// It takes a URL string as input and returns the modified URL string with "http://" prefix
func EnforceHTTP(url string) string {
	// Check if the URL starts with "http"
	if url[:4] != "http" {
		// Add the "http://" prefix to the URL
		return "http://" + url
	}
	// If the URL already starts with "http", return the URL as it is
	return url
}

// RemoveDomainError is a function that removes the domain name from the URL string and checks if it is equal to the DOMAIN environment variable
// It takes a URL string as input and returns a boolean value
func RemoveDomainError(url string) bool {

	// Check if the input URL is equal to the DOMAIN environment variable
	if url == os.Getenv("DOMAIN") {
		// Return false if the input URL is equal to the DOMAIN environment variable
		return false
	}

	// Remove the "http://" and "https://" prefixes from the URL
	newURL := strings.Replace(url, "http://", "", 1)
	newURL = strings.Replace(newURL, "https://", "", 1)

	// Remove the "www." prefix from the URL.
	newURL = strings.Replace(newURL, "www.", "", 1)

	// Split the URL at the first occurrence of "/" and get the first part
	newURL = strings.Split(newURL, "/")[0]

	// Check if the modified URL is equal to the DOMAIN environment variable
	if newURL == os.Getenv("DOMAIN") {
		// Return false if the modified URL is equal to the DOMAIN environment variable
		return false
	}

	// Return true if the modified URL is not equal to the DOMAIN environment variable
	return true
}
