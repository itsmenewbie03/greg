package scraper

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const baseURL = "https://wheredoestheanimeleaveoff.com/"

// GetMangaInfo scrapes the website to find manga continuation points for a given anime.
// It first tries to scrape with the original title. If that fails, it uses AniList
// to find a parent series and retries with the parent's title.
func GetMangaInfo(animeTitle string) (string, error) {
	info, err := scrapeMangaInfoInternal(animeTitle)
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no information found") && !strings.Contains(err.Error(), "Could not extract specific") {
			return "", err
		}
	}

	if info != "" && (err == nil || !strings.Contains(strings.ToLower(err.Error()), "no information found")) {
		return info, nil
	}

	// If the initial scrape yielded no results, try to find a parent series via AniList.
	parentTitle, parentErr := getAnimeParentSeries(animeTitle)
	if parentErr != nil {
		// Log the error but don't fail the whole operation, since we can still return the original (lack of) info.
		// For debugging purposes, we can print it.
		// fmt.Printf("AniList parent search failed: %v\n", parentErr)
		return "No information found.", nil
	}

	if parentTitle != "" {
		// If a parent title is found, try scraping again with the parent title.
		info, err = scrapeMangaInfoInternal(parentTitle)
		if err == nil && info != "" {
			// Prepend a notice that the information is for the parent series.
			return fmt.Sprintf("Displaying results for '%s' (parent series of '%s'):\n\n%s", parentTitle, animeTitle, info), nil
		}
	}

	// If fallback also fails, return the original "not found" message.
	return "No information found.", nil
}

// scrapeMangaInfoInternal performs the actual scraping for a given anime title.
func scrapeMangaInfoInternal(animeTitle string) (string, error) {
	searchURL := baseURL + "?s=" + url.QueryEscape(animeTitle)

	client := &http.Client{}
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/105.0.0.0 Safari/537.36")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("search request failed with status code: %d", res.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", err
	}

	var articleURL string
	// Adjusted selector to be more robust
	doc.Find("article h2.entry-title a").EachWithBreak(func(i int, s *goquery.Selection) bool {
		href, exists := s.Attr("href")
		if exists {
			// Heuristic: check if the URL slug contains a simplified version of the anime title.
			// This is brittle but better than nothing.
			slugTitle := strings.ReplaceAll(strings.ToLower(animeTitle), " ", "-")
			// Remove special characters for better matching
			re := regexp.MustCompile(`[^a-z0-9-]`)
			slugTitle = re.ReplaceAllString(slugTitle, "")
			if strings.Contains(href, slugTitle) {
				articleURL = href
				return false // break
			}
		}
		return true // continue
	})

	if articleURL == "" {
		// Fallback to the very first result if our heuristic fails.
		articleURL, _ = doc.Find("article h2.entry-title a").First().Attr("href")
		if articleURL == "" {
			return "", fmt.Errorf("no information found")
		}
	}

	// Scrape the article page.
	req, err = http.NewRequest("GET", articleURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/105.0.0.0 Safari/537.36")

	res, err = client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != 200 {
		return "", fmt.Errorf("article request failed with status code: %d", res.StatusCode)
	}

	doc, err = goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	doc.Find(".entry-content p, .entry-content li").Each(func(i int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}

		lowerText := strings.ToLower(text)

		// Skip common introductory or irrelevant phrases
		if strings.Contains(lowerText, "want to continue") ||
			strings.Contains(lowerText, "wish to continue") ||
			strings.Contains(lowerText, "you can purchase the manga") ||
			strings.HasPrefix(lowerText, "as an amazon associate") ||
			strings.HasSuffix(lowerText, "start at:") {
			return
		}

		// More aggressive filtering for keywords
		if strings.Contains(lowerText, "chapter") ||
			strings.Contains(lowerText, "volume") ||
			strings.Contains(lowerText, "start reading from") {
			// Fix missing spaces after periods
			re := regexp.MustCompile(`\.([A-Z])`)
			text = re.ReplaceAllString(text, ". $1")

			result.WriteString(text)
			result.WriteString("\n\n")
		}
	})

	if result.Len() == 0 {
		return "", fmt.Errorf("could not extract specific chapter/volume information, try reading the article on the website")
	}

	return strings.TrimSpace(result.String()), nil
}
