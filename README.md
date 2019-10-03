# nicky
Scraper for nickelodeon website.

Note: since HTTPS support on nickelodeon websites is broken (wrong CN), i've opted to stay on HTTP.

## Usage
```
go run main.go -site=nickelodeon.be -show=shows/474-spongebob -path=/Media/
```

## Requirements
* Go
* rtmpdump

## Plex Scanning
nicky will try running the `Plex Media Scanner` if there were episodes downloaded. Set the path to the scanner with `-scanner="/Plex/Plex Media Scanner"` and the section id with `-section=4`.
