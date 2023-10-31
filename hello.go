package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/gocolly/colly"
	"github.com/gorilla/websocket"
)

type FakeUserAgentsResponse struct {
	Result []string `json:"result"`
}

func GetUserAgentList(fakeUserAgentsBuffer chan string) {
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
		var fakeUserAgentResponse FakeUserAgentsResponse
		json.NewDecoder(resp.Body).Decode(&fakeUserAgentResponse)
		for i := 0; i < len(fakeUserAgentResponse.Result); i += 1 {
			fakeUserAgentsBuffer <- fakeUserAgentResponse.Result[i]
		}
	}
}

func getQuotePriceTest(quote string, c *colly.Collector, userAgent string, version int) float32 {
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	var quotePrice float32

	c.SetRequestTimeout(25 * time.Second)
	c.Limit(&colly.LimitRule{RandomDelay: 500 * time.Millisecond})

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", userAgent)
		fmt.Println("Visiting ", r.AbsoluteURL(string(r.URL.String())))
	})
	quoteSelector := fmt.Sprintf(`fin-streamer[data-symbol=%v][data-field=regularMarketPrice]`, quote)
	timeSelector := fmt.Sprintf(`div[id=quote-market-notice]`)
	c.OnResponse(func(r *colly.Response) {
		fmt.Println(r.StatusCode)
	})
	c.OnHTML(timeSelector, func(r *colly.HTMLElement) {
		fmt.Println(r.Text)
	})
	c.OnHTML(quoteSelector, func(r *colly.HTMLElement) {
		qprice, err := strconv.ParseFloat(r.Text, 32)
		if err != nil {
			fmt.Println("Error when parsing float price.")
			quotePrice = -1
		} else {
			quotePrice = float32(qprice)
			fmt.Println("HTML quote price", quotePrice)
		}
	})
	fmt.Println("Calling Visit on URL ", URL, " ", version)
	err := c.Visit(URL)
	if err != nil {
		fmt.Println("An error occured: for version", version, " => ", err)
		return -1
	}
	fmt.Println("Done ", version)
	return quotePrice
}

func getQuotePrice(quote string, c *colly.Collector, userAgent string) float32 {
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	var quotePrice float32
	var wg sync.WaitGroup
	wg.Add(1)
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", userAgent)
		fmt.Println("Visiting ", r.AbsoluteURL(string(r.URL.String())))
	})
	quoteSelector := fmt.Sprintf(`fin-streamer[data-symbol=%v][data-field=regularMarketPrice]`, quote)
	c.OnResponse(func(r *colly.Response) {
		fmt.Println(r.StatusCode)
	})
	c.OnHTML(quoteSelector, func(r *colly.HTMLElement) {
		defer wg.Done()
		qprice, err := strconv.ParseFloat(r.Text, 32)
		if err != nil {
			fmt.Println("Error when parsing float price.")
			quotePrice = -1
		} else {
			quotePrice = float32(qprice)
			fmt.Println("HTML quote price", quotePrice)
		}
	})
	fmt.Println("Calling Visit on URL ", URL)
	c.Visit(URL)
	wg.Wait()
	return quotePrice
}

var fakeUserAgents = make(chan string, 100)

var c *colly.Collector = colly.NewCollector(
	colly.AllowURLRevisit(),
)

func handleStringQuotePrice(w http.ResponseWriter, r *http.Request) {
	quoteName := r.URL.Query().Get("quote")
	// collector := c.Clone()
	userAgent := <-fakeUserAgents
	fmt.Println("Calling getQuotePrice with", quoteName, userAgent)
	price := getQuotePrice(quoteName, c, userAgent)
	fmt.Fprintf(w, "%v", price)
}

/*
*
* Get the next count prices from quote <quotename>
*
 */
func GetNextCountPrices(count int, quotename string, c *colly.Collector) {
	allPricesBuffer := make(chan float32, count)
	for i := 0; i < count; i += 1 {
		go func(version int) {
			collector := c.Clone()
			// collector := *colly.NewCollector(colly.AllowURLRevisit())
			allPricesBuffer <- getQuotePriceTest(quotename, collector, <-fakeUserAgents, version)
		}(i)
		time.Sleep(500 * time.Millisecond)
	}
	i := 0
	result := []float32{}
	for price := range allPricesBuffer {
		fmt.Println("Price: ", price, " ", i)
		result = append(result, price)
		i += 1
		if i >= count {
			close(allPricesBuffer)
			break
		}
	}

	fmt.Println()
}

var upgrader websocket.Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func Reader(conn *websocket.Conn) {
	/**
	  The reader essentialy reads the readBuffer continously till int encounters errors or the connection closes.
	*/

	for {
		// Message Type || message (p) || error
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error occured when reading message. Breaking connection")
			fmt.Println(err)
			break
		}
		fmt.Println(string(p))
		// Now that we were able to print the client message, return it back using the write message function
		if err := conn.WriteMessage(messageType, p); err != nil {
			fmt.Println("ERR when writing back!", err)
		}
	}
}

func wsEndpoint(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil) // Attempt to upgrade the request. This involves checking the client headers to check if the necessary headers are set.
	if err != nil {
		fmt.Println("Error occured when upgrading connection!", err)
		return
	}
	fmt.Println("Client connected!")
	Reader(ws)
}

func main() {
	http.Handle("/quotePrice", http.HandlerFunc(handleStringQuotePrice))
	http.Handle("/ws", http.HandlerFunc(wsEndpoint))
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
	go func() {
		for {
			// fmt.Println("Fake users list length ", len(fakeUserAgents))
			if len(fakeUserAgents) < 10 {
				fmt.Println("Getting more fake user agents.")
				GetUserAgentList(fakeUserAgents)
				fmt.Println("New Count: ", len(fakeUserAgents))
			}
		}
	}()
	// GetNextCountPrices(5, "AAPL", c)
	// GetNextCountPrices(1, "AAPL", c)

	// fmt.Println("Listening to port 8080!")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
