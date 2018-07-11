package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"
	"os"
	"path"

	"github.com/mattn/go-sqlite3"
)

var (
	db *sql.DB // functions will access this global variable to read and write to database
)

// Quote describes a specific moment in time for a cryptocurrency asset.
// All prices are quoted in USD. E.g Bitcoin's open, close, high, and low values would all be BTC -> USD.
type Quote struct {
	Time       int64   `json:"time"` // is a unix timestamp
	Close      float64 `json:"close"`
	High       float64 `json:"high"`
	Low        float64 `json:"low"`
	Open       float64 `json:"open"`
	VolumeFrom float64 `json:"volumefrom"`
	VolumeTo   float64 `json:"volumeto"`
}

// response we get from querying the API. Used to easily read the data.
type CryptoCompareResponse struct {
	Response          string  `json:"Response"`
	Type              int     `json:"Type"`
	Aggregated        bool    `json:"Aggregated"`
	Data              []Quote `json:"Data"` // this is what we're really interested in
	TimeTo            int     `json:"TimeTo"`
	TimeFrom          int     `json:"TimeFrom"`
	FirstValueInArray bool    `json:"FirstValueInArray"`
	ConversionType    struct {
		Type             string `json:"type"`
		ConversionSymbol string `json:"conversionSymbol"`
	} `json:"ConversionType"`
}

func initializeDB(path string) (*sql.DB, error) {
	// set database path and open a connection
	database, err := sql.Open("sqlite3", path)
	err = database.Ping()
	return database, err
}

// Gets the maximum number of historical quotes allowed (2000) from the CryptoCompare API. Quotes given are hourly.
func get(symbol string, time int64) []Quote {
	query := fmt.Sprintf("https://min-api.cryptocompare.com/data/histohour?fsym=%s&tsym=USD&limit=2000&aggregate=1&toTs=%v", symbol, time)
	res, err := http.Get(query)
	if err != nil {
		log.Fatal(err)
	}
	// remember to close body reader when we're done
	defer res.Body.Close()
	var info CryptoCompareResponse
	// parse JSON response and catch any errors
	if json.NewDecoder(res.Body).Decode(&info); err != nil {
		log.Print("There was an error parsing the response:", err)
	}
	// return only the slice of Quotes
	return info.Data
}

func createTable(symbol string) (sql.Result, error) {
	// command to create us a table
	command := fmt.Sprintf("CREATE TABLE \"%s\" (time INT UNIQUE, close FLOAT, high FLOAT, low FLOAT, open FLOAT, volume_from FLOAT, volume_to FLOAT)", symbol)
	// create the table
	result, err := db.Exec(command)
	if err != nil {
		log.Printf("Could not create database table for: %s", symbol)
		return result, err
	}
	// return nil error
	return result, nil
}

// Creates a table if it does not exist, and returns the "created" boolean.
func createTableIfNeeded(symbol string) (bool, error) {
	// dummy variable to check if the database table exists. Can't use blank identifier
	var dummy string
	query := fmt.Sprintf("SELECT name FROM sqlite_master WHERE type=\"table\" AND name=\"%s\"", symbol)
	err := db.QueryRow(query).Scan(&dummy)
	if err == sql.ErrNoRows {
		_, err := createTable(symbol)
		if err != nil {
			return false, err
		}
		return true, nil
	} else if err != nil {
		return false, err
	} else {
		// already exists
		return false, nil
	}
}

func isDummyQuote(quote Quote) bool {
	if (quote.Open == 0) && (quote.Close == 0) && (quote.High == 0) && (quote.Low == 0) {
		return true
	} else {
		return false
	}
}

// Resolves the path given to the command line flag and returns an absolute version.
func resolvePath(dbpath string) (string, error) {
	// determine if absolute or relative path
	if path.IsAbs(dbpath) {
		return dbpath, nil
	} else {
		// return resolved relative path
		wd, err := os.Getwd()
		dbpath = path.Join(wd, dbpath)
		return dbpath, err
	}
}

// Writes the given quotes to the database.
func writeToDB(quotes []Quote, symbol string) (Quote, error) {
	// if the table doesn't exist, we create it
	_, err := createTableIfNeeded(symbol)
	if err != nil {
		log.Printf("Table creation for %s failed", symbol)
		return Quote{}, err
	}
	// begin a transaction to lump together the quotes we are writing
	tx, err := db.Begin()
	if err != nil {
		return Quote{}, err
	}
	// loop through quotes and add them to the database
	for _, q := range quotes {
		// check quote not empty. Might get an empty quote halfway through
		if isDummyQuote(q) {
			break
		}
		// create placeholder query using the symbol we used
		query := fmt.Sprintf("INSERT INTO \"%s\" (time, close, high, low, open, volume_from, volume_to) VALUES ($1, $2, $3, $4, $5, $6, $7)", symbol)
		_, err := tx.Exec(query, q.Time, q.Close, q.High, q.Low, q.Open, q.VolumeFrom, q.VolumeTo)
		// handle all cases of database errors
		if err != nil {
			driverErr, ok := err.(sqlite3.Error)
			if !ok {
				// if we couldn't convert for some reason, just return the error
				return Quote{}, err
			}
			// run through cases
			switch {
			case driverErr.ExtendedCode == 2067:
				// this indicates a UNIQUE constraint failed i.e writing duplicated data to DB
				log.Printf("Duplicate value for %v timestamp %v. Skipping...", symbol, q.Time)
				continue
			default:
				// generic error message
				log.Printf("Write to %s failed", symbol)
				return Quote{}, err
			}
		}
	}
	// commit transaction
	tx.Commit()
	earliest := quotes[0]
	return earliest, nil
}

// Continuously gets the historical data for a symbol until there is no more available.
func getHistoricalFor(symbol string, unixtime int64, donec chan string, errc chan error) {
	// get most recent data (for today)
	data := get(symbol, unixtime)
	// exit if our latest date has an open value of 0. The CryptoCompare API simply returns blank items with 0 values, so that is why we have to check.
	if isDummyQuote(data[len(data)-1]) {
		donec <- symbol
		return
	}
	// obtain the earliest date fetched
	earliest, err := writeToDB(data, symbol)
	if err != nil {
		errc <- err
		return
	}
	// set date for limit
	untilDate := earliest.Time - 1
	// inform progress
	//log.Printf("Fetched 2000 from %v", symbol)
	// wait a few milliseconds so that the API doesn't complain
	time.Sleep(500 * time.Millisecond)
	// recursively get historical again
	getHistoricalFor(symbol, untilDate, donec, errc)
}

func main() {
	// create and parse all command line flags
	dbpath := flag.String("dbpath", "./historical.db", "path to the database file where information will be stored.")
	flag.Parse()
	// resolve the path to an absolute one
	resPath, err := resolvePath(*dbpath)
	if err != nil {
		log.Fatal(err)
	}
	// establish connection to the database and check for errors
	db, err = initializeDB(resPath)
	if err != nil {
		log.Fatal(err)
	}
	// get all the command line arguments coming after the flag, which should be the symbols
	symbols := flag.Args()
	// create channels to receive on
	donec, errc := make(chan string), make(chan error)
	// go get each symbol's data concurrently
	for _, symbol := range symbols {
		log.Printf("Fetching historical data for: %v", symbol)
		go getHistoricalFor(symbol, time.Now().Unix(), donec, errc)
	}
	// this will block while it waits for channels to become available and send data
	for i := 0; i < len(symbols); i++ {
		select {
		case done := <-donec:
			log.Printf("Got all data for: %s", done)
		case err := <-errc:
			log.Println(err)
		}
	}
}
