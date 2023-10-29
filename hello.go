package main

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"text/template"

	"github.com/gocolly/colly"
)

func getQuotePrice(quote string, c *colly.Collector, userAgent string) float32 {
	URL := fmt.Sprintf("https://finance.yahoo.com/quote/%v", quote)
	var quotePrice float32
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("User-Agent", userAgent)
		fmt.Println("Visiting ", r.AbsoluteURL(string(r.URL.String())))
	})

	c.OnHTML(`fin-streamer[data-symbol=DAL][data-field=regularMarketPrice]`, func(r *colly.HTMLElement) {
		qprice, err := strconv.ParseFloat(r.Text, 32)
		if err != nil {
			fmt.Println("Error when parsing float price.")
			quotePrice = -1
		} else {
			quotePrice = float32(qprice)
		}
	})
	c.Visit(URL)
	return quotePrice
}

func main() {
	c := colly.NewCollector()

	userAgent := "1 Mozilla/5.0 (iPad; CPU OS 12_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"

	quotePrice := getQuotePrice("DAL", c, userAgent)
	http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.ParseFiles("template.html")
		if err != nil {
			fmt.Fprintf(w, "505 error")
		}

		data := struct {
			Message string
		}{
			Message: fmt.Sprintf("This page is still in construction but the current quotePrice is %v", quotePrice),
		}

		err = tmpl.Execute(w, data)

		if err != nil {
			fmt.Fprintf(w, "505 error 2")
		}
	}))

	fmt.Println("Listening to port 8080!")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
