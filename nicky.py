#!/usr/bin/env python3

import sys
from urllib.request import urlopen
from bs4 import BeautifulSoup
import subprocess
import shlex
import os.path
import shutil

def downloadPage( url ):
	return urlopen(url).read().decode('utf-8')

def parsePage( content ):
	return BeautifulSoup(content, 'html.parser')

def extractEpisodes( html ):
	episodes = []

	playlist = html.find("ol", {"class": "playlist"})

	for item in playlist.find_all('li'):
		try:
			if item['data-category'] != "fullepisode":
				continue

			id = item['data-item-id']
			title = item.find('p', {"class": "subtitle"}).getText()
			url = item.find('a')['href']

			episodes.append({
				'id': id,
				'title': title,
				'url': url
			})
		except:
			continue

	return episodes

def downloadStream(source, dest):
	tempFile = "sponge.temp"
	try:
		process = subprocess.Popen("/usr/local/bin/rtmpdump --url " + shlex.quote(source) + " -o  " + tempFile, shell=True, stdout=subprocess.PIPE)
		process.wait()
		success = (process.returncode == 0)

		if success is True:
			shutil.move(tempFile, dest)
	finally:
		if os.path.isfile(tempFile):
			shutil.remove(tempFile)

	return success

def findBestRelease(downloadPlaylist):
	renditions = downloadPlaylist.find_all('rendition')
	bestRendition = None
	for rendition in renditions:
		if bestRendition is None or (int(rendition['bitrate']) > int(bestRendition['bitrate'])):
			bestRendition = rendition
			
	if bestRendition is None:
		return None
	
	print("Best version has bitrate of " + bestRendition['bitrate'])
	return bestRendition.find('src').getText()

scannerBin = "/Applications/Plex Media Server.app/Contents/MacOS/Plex Media Scanner"

if len(sys.argv) is not 4:
	print("Usage: " + sys.argv[0] + " <showname> <destPath> <pageUrl>")
	sys.exit(1)

scriptName, showName, destPath, pageUrl = sys.argv

parsedHtml = parsePage(downloadPage(pageUrl))
episodes = extractEpisodes(parsedHtml)

needScan = False

for episode in episodes:
	print("Parsing '" + episode['title'] + "'")
	parsedHtml = parsePage(downloadPage(episode['url']))

	numbering = parsedHtml.find('h6', {'class': 'season-episode'}).getText()
	numbering = numbering.replace('Seizoen ', '')
	numbering = numbering.replace('- Aflevering ', '')
	season, epi = numbering.strip().split(' ')
	episode['numbering'] = "S" + season.zfill(2) + "E" + epi.zfill(2)


	destinationPath = destPath + showName + " - " + episode['numbering'] + ".mp4"
	if os.path.isfile(destinationPath):
		print("File already exists: " + destinationPath)
		continue

	playerWrapper = parsedHtml.find('div', {'class': 'player-wrapper'})
	playlistUrl = playerWrapper['data-mrss']
	
	parsedPlaylist = parsePage(downloadPage(playlistUrl))
	downloadPlaylistUrl = parsedPlaylist.find('media:content')['url']
	downloadPlaylist = parsePage(downloadPage(downloadPlaylistUrl))

	rtmpUrl = findBestRelease(downloadPlaylist)
	if rtmpUrl is None:
		print("WARNING: skipping since we didn't find a useable stream")
		continue

	print("Downloading to " + destinationPath)
	if downloadStream(rtmpUrl, destinationPath) is False:
		print("Error: could not download episode")

	needScan = True
	print("downloaded!")

if needScan is True:
	print("Downloaded episodes, so scanning with Plex")
	proc = subprocess.Popen([scannerBin, "--verbose", "--section", "4", "--scan", "--directory", destPath])
	proc.wait()

