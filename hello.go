package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"text/template"
	"time"

	"github.com/gocolly/colly"
)

type FakeUserAgentsResponse struct {
	Result []map[string]string `json:"result"`
}

type FakeUserAgentsResponseNew struct {
	Result []string `json:"result"`
}

func getFakeUserAgents() {
	scrapeopsAPIKey := "7a23f97c-aada-4964-9e1c-cf16b3dfc762"
	endpoint := "http://headers.scrapeops.io/v1/user-agents?api_key=" + scrapeopsAPIKey

	req, _ := http.NewRequest("GET", endpoint, nil)
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err == nil && resp.StatusCode == 200 {
		var fakeUserAgentResponse FakeUserAgentsResponse = FakeUserAgentsResponse{}
		err = json.NewDecoder(resp.Body).Decode(&fakeUserAgentResponse)
		if err != nil {
			fmt.Println("err during decode process.")
		}
		for _, usa := range fakeUserAgentResponse.Result {
			fmt.Println(usa)
		}
	}
}

func GetUserAgentList() {
	// ScrapeOps User-Agent API Endpint
	scrapeopsAPIKey := "7a23f97c-aada-4964-9e1c-cf16b3dfc762"
	scrapeopsAPIEndpoint := "http://headers.scrapeops.io/v1/user-agents?api_key=" + scrapeopsAPIKey

	req, _ := http.NewRequest("GET", scrapeopsAPIEndpoint, nil)
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make Request
	resp, err := client.Do(req)
	if err == nil && resp.StatusCode == 200 {
		defer resp.Body.Close()
		// Convert Body To JSON
		var fakeUserAgentResponse FakeUserAgentsResponseNew
		json.NewDecoder(resp.Body).Decode(&fakeUserAgentResponse)
		fmt.Println(fakeUserAgentResponse.Result)
	}
}

func getQuotePrice(quote string, c *colly.Collector, userAgent string) float32 {
	fmt.Println("Quote is ", quote)
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	var quotePrice float32
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", userAgent)
		fmt.Println("Visiting ", r.AbsoluteURL(string(r.URL.String())))
	})
	quoteSelector := fmt.Sprintf(`fin-streamer[data-symbol=%v][data-field=regularMarketPrice]`, quote)

	c.OnHTML(quoteSelector, func(r *colly.HTMLElement) {
		qprice, err := strconv.ParseFloat(r.Text, 32)
		if err != nil {
			fmt.Println("Error when parsing float price.")
			quotePrice = -1
		} else {
			quotePrice = float32(qprice)
		}
	})
	c.Visit(URL)
	fmt.Println(quotePrice)
	return quotePrice
}

func handleStringQuotePrice(w http.ResponseWriter, r *http.Request) {
	c := colly.NewCollector()

	quoteName := r.URL.Query().Get("quote")
	fmt.Printf("quote name is ", quoteName)

	userAgent := "1 Mozilla/5.0 (iPad; CPU OS 12_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
	price := getQuotePrice(quoteName, c, userAgent)
	fmt.Fprintf(w, "%v", price)
}

func main() {
	http.Handle("/quotePrice", http.HandlerFunc(handleStringQuotePrice))

	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Request URL is ", r.URL)
		tmpl, err := template.ParseFiles("template.html")
		if err != nil {
			fmt.Fprintf(w, "505 error")
		}

		finalMessage := "This page is still under construction!"
		data := struct {
			Message string
		}{
			Message: finalMessage,
		}

		err = tmpl.Execute(w, data)

		if err != nil {
			fmt.Fprintf(w, "505 error 2")
		}
	}))

	GetUserAgentList()
	fmt.Println("Listening to port 8080!")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
