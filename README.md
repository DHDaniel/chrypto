# Chrypto

Chrypto is a command-line utility tool written in Go that allows you to quickly obtain all the available historical data for a wide range of cryptocurrencies.

It obtains the asset movement information (open, high, low, close, and volumes) along with the date for hourly intervals since the conception of the cryptocurrency. For example, the utility could retrieve all the hourly historical price movements for Bitcoin since its conception.

The information is stored in an SQLite database, with a separate table created for each symbol. Each table contains all the hourly entries, with fields named **time, close, high, low, open, volume_from, and volume_to**. All of these fields correspond to the fields described in the [CryptoCompare API](https://min-api.cryptocompare.com/).

All prices are quoted in US dollars.

## Installation
Get the repository using `go get`, and then install.
```bash
$ go get github.com/DHDaniel/chrypto
$ cd $GOPATH/src/github.com/DHDaniel/chrypto
$ go install
```
Once installed, there should be a binary installed in your `$GOPATH/bin` folder named `chrypto`.

## Usage
Using the chrypto utility is very straightforward. All you need to provide is the list of symbols you want to retrieve data for, and an optional flag containing the location where you want to store the data.

### Example
This retrieves all the available data for Bitcoin and Ethereum.
```bash
$ chrypto BTC ETH
```

### Flags
The only flag that is available is the `dbpath` flag. It should be a path to the file that the script should store its data in. It defaults to `./historical.db`.
```bash
$ chrypto -dbpath="./path/to/my/database.db" BTC ETH
```
