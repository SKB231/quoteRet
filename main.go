package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

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

const quoteCacheTTL = 10 * time.Minute

type quoteCacheEntry struct {
	price     string
	timestamp time.Time
	expiresAt time.Time
}

var (
	quoteCache   = make(map[string]quoteCacheEntry)
	quoteLocks   = make(map[string]*sync.Mutex)
	quoteCacheMu sync.Mutex
)

func getQuoteLock(quote string) *sync.Mutex {
	quoteCacheMu.Lock()
	defer quoteCacheMu.Unlock()

	lock, ok := quoteLocks[quote]
	if !ok {
		lock = &sync.Mutex{}
		quoteLocks[quote] = lock
	}
	return lock
}

func getQuotePriceAndTime(quote string, userAgent string, requestTime time.Time) (string, string) {
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	reqUrl, err := url.Parse(URL)
	if err != nil {
		fmt.Println("Error with URL: ", err)
	}
	req := http.Request{
		Method: "GET",
		Header: map[string][]string{
			"User-Agent": []string{userAgent},
		},
		URL: reqUrl,
	}
	resp, err := http.DefaultClient.Do(&req)
	if err != nil {
		fmt.Println("Link Parser error: ", err)
		return "", ""
	}
	defer resp.Body.Close()
	res, err := linkParser.Parse(resp.Body, quote)

	if err != nil {
		fmt.Println("Link Parser error: ", err)
		return "", ""
	}

	return res.Price, requestTime.Format(time.RFC3339)
}

func getQuotePrice(quote string, requestTime time.Time) (string, string) {
	quote = strings.ToUpper(strings.TrimSpace(quote))
	if quote == "" {
		return "", requestTime.Format(time.RFC3339)
	}

	quoteCacheMu.Lock()
	cached, ok := quoteCache[quote]
	if ok && requestTime.Before(cached.expiresAt) {
		quoteCacheMu.Unlock()
		return cached.price, cached.timestamp.Format(time.RFC3339)
	}
	if ok {
		delete(quoteCache, quote)
	}
	quoteCacheMu.Unlock()

	quoteLock := getQuoteLock(quote)
	quoteLock.Lock()
	defer quoteLock.Unlock()

	quoteCacheMu.Lock()
	cached, ok = quoteCache[quote]
	if ok && requestTime.Before(cached.expiresAt) {
		quoteCacheMu.Unlock()
		return cached.price, cached.timestamp.Format(time.RFC3339)
	}
	if ok {
		delete(quoteCache, quote)
	}
	quoteCacheMu.Unlock()

	userAgent := <-fakeUserAgents
	fmt.Println("Calling getQuotePrice with", quote, userAgent)
	price, timestamp := getQuotePriceAndTime(quote, userAgent, requestTime)
	if price == "" {
		return price, timestamp
	}

	quoteCacheMu.Lock()
	quoteCache[quote] = quoteCacheEntry{
		price:     price,
		timestamp: requestTime,
		expiresAt: requestTime.Add(quoteCacheTTL),
	}
	quoteCacheMu.Unlock()

	return price, timestamp
}

var fakeUserAgents = make(chan string, 100)

func handleStringQuotePrice(w http.ResponseWriter, r *http.Request) {
	quoteName := r.URL.Query().Get("quote")
	requestTime := time.Now()
	// collector := c.Clone()
	price, timestamp := getQuotePrice(quoteName, requestTime)
	fmt.Fprintf(w, "%v at %v", price, timestamp)
}

func cleanupQuoteCache() {
	ticker := time.NewTicker(quoteCacheTTL)
	defer ticker.Stop()

	for now := range ticker.C {
		quoteCacheMu.Lock()
		for quote, entry := range quoteCache {
			if !now.Before(entry.expiresAt) {
				delete(quoteCache, quote)
			}
		}
		quoteCacheMu.Unlock()
	}
}

type interval string

const (
	WEEK_INTEVAL   interval = "1wk"
	MONTH_INTERVAL interval = "1mo"
	DAY_INTERVAL   interval = "1d"
)

// func getCSVData(quote string, startTime string, endTime string, limit int, userAgent string) (string, error) {
// 	reqUrl := fmt.Sprintf("https://query1.finance.yahoo.com/v7/finance/download/%v?period1=%v&period2=%v&interval=%v", quote, startTime, endTime, hopTime)
// 	urlObj, err := url.Parse(reqUrl)

// 	if err != nil {
// 		fmt.Printf("Error occured when retriving historical data %v \n", err)
// 		return "", err
// 	}

// 	req := http.Request{
// 		Method: "GET",
// 		Header: map[string][]string{
// 			"User-Agent": []string{userAgent},
// 		},
// 		URL: urlObj,
// 	}

// 	resp, err := http.DefaultClient.Do(&req)
// 	if err != nil {
// 		fmt.Printf("Error occured when retriving historical data %v \n", err)
// 		return "", err
// 	}
// 	data, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		fmt.Printf("Error occured when retriving historical data %v \n", err)
// 		return "", err
// 	}

// 	return string(data), nil
// }

func getCSVData(quote string, startDate, endDate, limit string, userAgent string) (string, error) {
	// Generate a random number between 1 and 30
	randomNum := rand.Intn(30) + 1

	// Format the URL to match the Nasdaq API structure
	reqUrl := fmt.Sprintf("https://api.nasdaq.com/api/quote/%v/historical?assetclass=stocks&fromdate=%v&todate=%v&limit=%v&random=%v",
		quote, startDate, endDate, limit, randomNum)

	urlObj, err := url.Parse(reqUrl)
	if err != nil {
		fmt.Printf("Error occurred when retrieving historical data: %v \n", err)
		return "", err
	}

	// Build the HTTP request
	req := http.Request{
		Method: "GET",
		Header: map[string][]string{
			"User-Agent": {userAgent},
		},
		URL: urlObj,
	}

	// Execute the HTTP request
	resp, err := http.DefaultClient.Do(&req)
	if err != nil {
		fmt.Printf("Error occurred when retrieving historical data: %v \n", err)
		return "", err
	}
	defer resp.Body.Close()

	// Read the response body
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error occurred when retrieving historical data: %v \n", err)
		return "", err
	}

	// Return the data as a string
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
	var dayCount int64 = ((pi / 3600.0) / 24.0)

	if dayCount > 365*3 {
		unixTime = time.Unix(31556926*3, 0)
		dayCount = 365 * 3
	}

	nowTime := time.Now()
	prevTime := nowTime.Sub(unixTime)
	nowTimeStr := fmt.Sprintf("%v", nowTime.Unix())
	prevTimeStr := fmt.Sprintf("%v", int(prevTime.Seconds()))
	// timePeriod := MONTH_INTERVAL
	// if dayCount < 32 {
	// 	timePeriod = DAY_INTERVAL
	// } else if dayCount < 225 {
	// 	timePeriod = WEEK_INTEVAL
	// }
	//fmt.Println(timePeriod, dayCount)
	//fmt.Println("Sending: ", prevTimeStr, nowTimeStr, timePeriod)
	// Convert Unix timestamps to time.Time format
	s1, e1 := strconv.Atoi(prevTimeStr)
	s2, e2 := strconv.Atoi(nowTimeStr)
	//limitArg, e3 :=
	if e1 != nil || e2 != nil {
		w.Write([]byte("Error occured with historical data retrival! " + e1.Error() + " or " + e2.Error()))
		return
	}
	time1 := time.Unix(int64(s1), 0).UTC()
	time2 := time.Unix(int64(s2), 0).UTC()

	// Format the timestamps to a human-readable format
	// fmt.Println("Timestamp 1:", time1.Format("2006-01-02"))

	csvData, err := getCSVData(quote, time1.Format("2006-01-02"), time2.Format("2006-01-02"), strconv.Itoa(int(dayCount)), <-fakeUserAgents)
	if err != nil {
		w.Write([]byte("Error occured with historical data retrival! " + err.Error()))
		return
	}
	var returnData struct {
		Data struct {
			TradesTable struct {
				Rows []struct {
					Date   string `json:"date"`
					Close  string `json:"close"`
					Volume string `json:"volume"`
					Open   string `json:"open"`
					High   string `json:"high"`
					Low    string `json:"low"`
				} `json:"rows"`
			} `json:"tradesTable"`
		} `json:"data"`
	}

	returnCSV := make([]struct {
		UnixTime string `json:"time"`
		Value    string `json:"value"`
	}, 0)

	err = json.Unmarshal([]byte(csvData), &returnData)
	if err != nil {
		w.Write([]byte("Error occured with historical data retrival! " + err.Error()))
		return
	}
	// Process each row to convert date to Unix time and extract close value
	for _, val := range returnData.Data.TradesTable.Rows {
		// Parse the date to time.Time object
		//date, err := time.Parse("01/02/2006", val.Date)
		//if err != nil {
		//	fmt.Printf("Error parsing date: %v\n", err)
		//	continue
		//}

		//// Convert to Unix timestamp
		//unixTime := date.Unix()

		// Clean up the "close" value by removing the "$" sign
		closeValue := strings.TrimPrefix(val.Close, "$")

		// Append to the returnCSV slice
		returnCSV = append(returnCSV, struct {
			UnixTime string `json:"time"`
			Value    string `json:"value"`
		}{
			//UnixTime: fmt.Sprintf("%d", unixTime),
			UnixTime: val.Date,
			Value:    closeValue,
		})
	}

	// Convert returnCSV to CSV format and write to response
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment;filename=data.csv")
	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write header
	csvWriter.Write([]string{"time", "value"})

	// Write the rows
	for _, row := range returnCSV {
		csvWriter.Write([]string{row.UnixTime, row.Value})
	}

	// Print debug info
	//fmt.Println("Received CSV Data:")
	for _, row := range returnCSV {
		fmt.Printf("Time: %v, Value: %v\n", row.UnixTime, row.Value)
	}
}

func allowOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Only allow 2 URLs. Current origin and gtequityResearch.
		next.ServeHTTP(w, r)
	})
}

func main() {
	http.Handle("/quotePrice", allowOrigin(http.HandlerFunc(handleStringQuotePrice)))
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
			time.Sleep(10 * time.Second)
		}
	}()
	go cleanupQuoteCache()

	listenTo := os.Getenv("ADDRESS") + ":" + os.Getenv("PORT")
	if listenTo == ":" {
		//Debug mode
		listenTo = ":3000"
	}
	fmt.Println(listenTo == "", len(listenTo))
	fmt.Println("Listening to ", listenTo)
	log.Fatal(http.ListenAndServe(listenTo, nil))
}
