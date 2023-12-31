package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
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
	scrapeopsAPIKey := os.Getenv("SCRAPE_OPS_KEY")
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

func getQuotePriceAndTime(quote string, c *colly.Collector, userAgent string, version int) (float32, string) {
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	var quotePrice float32
	var quoteTime string
	c.SetRequestTimeout(25 * time.Second)
	c.Limit(&colly.LimitRule{RandomDelay: 500 * time.Millisecond})

	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", userAgent)
		// fmt.Println("Visiting ", r.AbsoluteURL(string(r.URL.String())))
	})
	quoteSelector := fmt.Sprintf(`fin-streamer[data-symbol=%v][data-field=regularMarketPrice]`, quote)
	timeSelector := fmt.Sprintf(`div[id=quote-market-notice]`)
	c.OnHTML(timeSelector, func(r *colly.HTMLElement) {
		quoteTime = r.Text
	})
	c.OnHTML(quoteSelector, func(r *colly.HTMLElement) {
		qprice, err := strconv.ParseFloat(r.Text, 32)
		if err != nil {
			fmt.Println("Error when parsing float price.")
			quotePrice = -1
		} else {
			quotePrice = float32(qprice)
			// fmt.Println("HTML quote price", quotePrice)
		}
	})
	// fmt.Println("Calling Visit on URL ", URL, " ", version)
	err := c.Visit(URL)
	if err != nil {
		fmt.Println("An error occured: for version", version, " => ", err)
		return -1, ""
	}
	return quotePrice, quoteTime
}

// FOR SIMPLE HTTP CALLS
func getQuotePrice(quote string, c *colly.Collector, userAgent string) float32 {
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	var quotePrice float32
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", userAgent)
		fmt.Println("Visiting ", r.AbsoluteURL(string(r.URL.String())))
	})
	quoteSelector := fmt.Sprintf(`fin-streamer[data-symbol=%v][data-field=regularMarketPrice]`, quote)
	c.OnResponse(func(r *colly.Response) {
		fmt.Println(r.StatusCode)
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
	fmt.Println("Calling Visit on URL ", URL)
	c.Visit(URL)
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

type priceBufferType struct {
	quote string
	price float32
	time  string
}

/*
*
* Get the next count prices from quote <quotename>
*
 */
func GetNextCountPrices(count int, quotename string, c *colly.Collector, allPricesBuffer chan priceBufferType, quit chan bool) {
Loop:
	for i := 0; i < count; i += 1 {
		select {
		case <-quit:
			fmt.Println("STOP GETTING PRICES")
			break Loop
		default:
			go func(version int) {
				collector := c.Clone()
				// collector := *colly.NewCollector(colly.AllowURLRevisit())
				quotePrice, time := getQuotePriceAndTime(quotename, collector, <-fakeUserAgents, version)
				allPricesBuffer <- priceBufferType{quotename, quotePrice, time}
			}(i)
			time.Sleep(1500 * time.Millisecond)
		}
	}
	fmt.Println("DONE")
}

var upgrader websocket.Upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

const COUNT int = 1000 // The default value of the number of quoteprices to return from the GetNextCountPrices func

// TODO: have a quit channel to quit when connection is deleted.
func recieveQuotes(allPricesBuffer chan priceBufferType, quit chan bool, conn *websocket.Conn) {
	for {
		select {
		case nextPrice := <-allPricesBuffer:
			fmt.Println(nextPrice)
			conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("%v", nextPrice)))
		case <-quit:
			fmt.Println("QUIT REQUESTED")
			break
		}
	}
}

type ConnectionKey struct {
	Connection *websocket.Conn
	Quote      string
}

// connections map handling function. Make sure to use mutex properly and key check before retriving values.
var (
	connectionsMap map[ConnectionKey](chan bool) = make(map[ConnectionKey](chan bool))
	mu             sync.Mutex                    // Mutex to shared resource connectionsMap
)

func checkKeyExists(conn *websocket.Conn, quote string) bool {
	_, keyExists := connectionsMap[ConnectionKey{conn, quote}]
	return keyExists == true
}

func addKey(conn *websocket.Conn, quote string) chan bool {
	key := ConnectionKey{conn, quote}
	quitChannel := make(chan bool)
	connectionsMap[key] = quitChannel
	return quitChannel
}

func getValue(conn *websocket.Conn, quote string) chan bool {
	key := ConnectionKey{conn, quote}
	return connectionsMap[key]
}

func deletePair(conn *websocket.Conn, quote string) {
	key := ConnectionKey{conn, quote}

	delete(connectionsMap, key)
}

func Reader(conn *websocket.Conn) {
	/**
	  The reader essentialy reads the readBuffer continously till int encounters errors or the connection closes.
	*/
	defer conn.Close() // Gracefully delete the connection upon the end of the method.
	allPricesBuffer := make(chan priceBufferType, COUNT)
	quit := make(chan bool)

	go recieveQuotes(allPricesBuffer, quit, conn)

	stopAllRecieves := func(connectionToStop *websocket.Conn) {
		mu.Lock()
		for key, val := range connectionsMap {
			if key.Connection != connectionToStop {
				continue
			}
			quitChannel := val
			deletePair(key.Connection, key.Quote)
			quitChannel <- true
		}
		mu.Unlock()
	}

	defer stopAllRecieves(conn)
	/*
	  To keep track of all subscribed quotes, we keep track of thier respective quit channels in a map with the key being a struct containing the quotename and connection, and value being the associated quit channel pointer
	  Whenever the user requests to stop subscribing, we remove the key value pair and send to quit channel. This should stop the goruitne controlling the getQuotePrice for the associated company
	*/
Loop_Main:
	for {
		// Message Type || message (p) || error
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("Error occured when reading message. Breaking connection")
			fmt.Println(err)
			err := conn.Close()
			if err != nil {
				fmt.Println("Error when breaking connection, ", err)
			}
			break Loop_Main
		}
		fmt.Println(string(p))
		userMessage := string(p)
		splits := strings.Split(userMessage, "|") // Messages will be of 2 forms all of a single line. QUOTE|DAL or QUOTE|AAPL will start tracking the respective quote prices by giving second by second updates
		// To stop tracking quotes, the user should send the message DELETE|DAL or DELETE|AAAPL respectively to stop tracking the quotes.
		fmt.Println(splits)
		switch {
		case splits[0] == "QUOTE":
			mu.Lock()
			if checkKeyExists(conn, splits[1]) {
				fmt.Println("KEY already exists. Continuing")
				mu.Unlock()
				continue
			}
			// new key.
			quitChan := addKey(conn, splits[1])
			mu.Unlock()
			fmt.Println("Tracking quote ", splits[1], " of length ", len(splits[1]))
			go GetNextCountPrices(COUNT, splits[1], c, allPricesBuffer, quitChan)
			if err = conn.WriteMessage(messageType, []byte(fmt.Sprintf("Tracking quote %v", splits[1]))); err != nil {
				fmt.Println("Error when writing message! Breaking connection.")
				err = conn.Close()
				if err != nil {
					fmt.Println("Error when breaking connection.")
					fmt.Println(err)
				}
				break Loop_Main
			}
		case splits[0] == "DELETE":
			fmt.Printf("Stop tracking quote %v", splits[1])
			fmt.Println(connectionsMap, checkKeyExists(conn, splits[1]))
			mu.Lock()
			if !checkKeyExists(conn, splits[1]) {
				if err := conn.WriteMessage(messageType, []byte("The quote is not subscribed to.")); err != nil {
					mu.Unlock()
					continue
				}
			} else {
				quitChannel := getValue(conn, splits[1])
				quitChannel <- true
				deletePair(conn, splits[1])
				fmt.Println("DELETED THE PAIR", connectionsMap)
			}
			mu.Unlock()
			if err = conn.WriteMessage(messageType, []byte(fmt.Sprintf("Stop tracking quote %v", splits[1]))); err != nil {
				fmt.Println("Error when writing message! Breaking connection.")
				err = conn.Close()
				if err != nil {
					fmt.Println("Error when breaking connection.")
					fmt.Println(err)
				}
			}
		default:
			fmt.Printf("Unknown option requested terminationg connection")
			err = conn.Close()
			if err != nil {
				fmt.Println("Error when breaking connection.")
				fmt.Println(err)
			}
			break Loop_Main
		}
		// Now that we were able to print the client message, return it back using the write message function
		if err != nil {
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
	go Reader(ws)
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

		data := struct{}{}

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
	listenTo := os.Getenv("ADDRESS") + ":" + os.Getenv("PORT")
	log.Fatal(http.ListenAndServe(listenTo, nil))
}
