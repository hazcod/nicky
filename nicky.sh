#!/usr/bin/env bash

baseUrl="http://www.nickelodeon.nl"

episodes=$(curl -s https://www.nickelodeon.nl/shows/76ypv4/spongebob | grep -Eo '(\/episodes\/[a-zA-Z0-9.,-\/+]*)')

for episode in ${episodes[@]}; do
	epiUrl="${baseUrl}${episode}"

	if ! curl -s "${epiUrl}"; then
		echo "Unable to download episode: ${epiUrl}"
		continue
	fi

	echo "Downloading ${epiUrl}"

	numbering="$(echo "$epiUrl" | grep -Eo '(seasonnumber\-\d+\-afl\-\d+)' | sed 's/seasonnumber\-/SE/' | sed 's/\-afl-/E/')"
	echo "This will be ${numbering}"

  youtube_dlc \
  "${epiUrl}" \
  --no-check-certificate \
  --download-archive ~/.sponge \
  -r 5m \
  -f best \
  -c \
  -w \
  --add-metadata \
  -i \
  --output "/My Series/Spongebob Squarepants/Spongebob Squarepants - ${numbering}.%(ext)s"
done


echo "Scanning files"
/Applications/Plex\ Media\ Server.app/Contents/MacOS/Plex\ Media\ Scanner --section 2 --scan --directory "/My Series/Spongebob Squarepants/" --verbose
