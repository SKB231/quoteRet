package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/gorilla/websocket"

	"github.com/SKB231/quoteRet/linkParser"
)



type FakeUserAgentsResponse struct {
	Result []string `json:"result"`
}

func GetUserAgentList(fakeUserAgentsBuffer chan string) {
	// ScrapeOps User-Agent API Endpint
	scrapeopsAPIKey := os.Getenv("SCRAPE_OPS_KEY")
	scrapeopsAPIKey = "7a23f97c-aada-4964-9e1c-cf16b3dfc762"
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

func getQuotePriceAndTime(quote string,userAgent string, version int) (string, string) {
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	reqUrl, err := url.Parse(URL);
	if err != nil {
		fmt.Println("Error with URL: ", err);
	}
	req := http.Request{
		Method: "GET",
		Header: map[string][]string{
			"User-Agent": []string{userAgent},
		},
		URL: reqUrl,
	}
	resp, err := http.DefaultClient.Do(&req);
	if err != nil {
		fmt.Println("Link Parser error: ", err);
		return "", "";
	}
	res, err := linkParser.Parse(resp.Body, quote);

	if err != nil {
		fmt.Println("Link Parser error: ", err);
		return "", "";
	}

	return res.Price, res.Time
}


func getQuotePrice(quote string, userAgent string) (string, string) {
	//strings.ReplaceAll(quote, ".", "/.")
	resPrice, time := getQuotePriceAndTime(quote, userAgent, -1);
	return resPrice, time
}

var fakeUserAgents = make(chan string, 100)


func handleStringQuotePrice(w http.ResponseWriter, r *http.Request) {
	quoteName := r.URL.Query().Get("quote")
	// collector := c.Clone()
	userAgent := <-fakeUserAgents
	fmt.Println("Calling getQuotePrice with", quoteName, userAgent)
	price, time := getQuotePrice(quoteName, userAgent)
	fmt.Fprintf(w, "%v at %v", price, time)
}

type priceBufferType struct {
	quote string
	price string
	time  string
}

/*
*
* Get the next count prices from quote <quotename>
*
 */
func GetNextCountPrices(count int, quotename string, allPricesBuffer chan priceBufferType, quit chan bool) {
	fmt.Printf("Getting sequence for %v\n", quotename)
Loop:
	for i := 0; i < count; i += 1 {
		select {
		case <-quit:
			fmt.Println("STOP GETTING PRICES")
			break Loop
		default:
			go func(version int) {
				quotePrice, time := getQuotePriceAndTime(quotename, <-fakeUserAgents, version)
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
			go GetNextCountPrices(COUNT, splits[1], allPricesBuffer, quitChan)
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

type interval string

const (
	WEEK_INTEVAL interval = "1wk"
	MONTH_INTERVAL interval = "1mo"
	DAY_INTERVAL interval = "1d"
)

func getCSVData(quote string, startTime string, endTime string, hopTime interval, userAgent string) (string, error){
	reqUrl := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/download/%v?period1=%v&period2=%v&interval=%v", quote, startTime, endTime, hopTime)
	urlObj, err := url.Parse(reqUrl)
	
	if err != nil {
		fmt.Printf("Error occured when retriving historical data %v \n", err)
		return "", err
	}

	req := http.Request{
		Method: "GET",
		Header: map[string][]string{
			"User-Agent": []string{userAgent},
		},
		URL: urlObj,
	}

	resp, err := http.DefaultClient.Do(&req);
	if err != nil {
		fmt.Printf("Error occured when retriving historical data %v \n", err)
		return "", err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error occured when retriving historical data %v \n", err)
		return "", err
	}

	return string(data), nil
}

// Extracts the query arguments to return at max 3 years worth of data points
func getHistoricalData(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	quote := r.URL.Query().Get("quote")
	if period == "" || quote == "" {
		w.Write([]byte("Not enough arguments"))
		return
	}
	pi, err := strconv.ParseInt(period, 10, 64)
	if err != nil {
		w.Write([]byte("Time endpoints incorrect"))
		return
	}
	unixTime := time.Unix(pi, 0)
	var dayCount int64 = ((pi/3600.0)/24.0)

	if dayCount > 365*3 {
		unixTime = time.Unix(31556926*3, 0)
		dayCount = 365*3
	}
	

	nowTime := time.Now()
	prevTime := nowTime.Sub(unixTime)
	nowTimeStr := fmt.Sprintf("%v", nowTime.Unix())
	prevTimeStr := fmt.Sprintf("%v", int(prevTime.Seconds()))
	timePeriod := MONTH_INTERVAL
	if dayCount < 32 {
		timePeriod = DAY_INTERVAL
	} else if dayCount < 225 {
		timePeriod = WEEK_INTEVAL
	}
	fmt.Println(timePeriod, dayCount)

	csvData, err := getCSVData(quote, prevTimeStr, nowTimeStr, timePeriod, <- fakeUserAgents)
	if err != nil {
		w.Write([]byte("Error occured with historical data retrival! " + err.Error()))
		return
	}
	w.Write([]byte(csvData))
}

func allowOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Only allow 2 URLs. Current origin and gtequityResearch.
		next.ServeHTTP(w, r);
	})
}

func main() {
	http.Handle("/quotePrice", allowOrigin(http.HandlerFunc(handleStringQuotePrice)))
	http.Handle("/ws", allowOrigin(http.HandlerFunc(wsEndpoint)))
	http.Handle("/getHistorical", allowOrigin(http.HandlerFunc(getHistoricalData)))
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
			if len(fakeUserAgents) < 20 {
				fmt.Println("Getting more fake user agents.")
				GetUserAgentList(fakeUserAgents)
				fmt.Println("New Count: ", len(fakeUserAgents))
			}

			// Check again after 10 seconds otherwise server go BURRRR
			time.Sleep(10*time.Second)
		}
	}()
	// GetNextCountPrices(5, "AAPL", c)
	// GetNextCountPrices(1, "AAPL", c)

	listenTo := os.Getenv("ADDRESS") + ":" + os.Getenv("PORT")
	if listenTo == ":" {
		//Debug mode
		listenTo = ":3000"
	}
	fmt.Println(listenTo == "", len(listenTo))
	fmt.Println("Listening to ", listenTo)
	log.Fatal(http.ListenAndServe(listenTo, nil))
}
