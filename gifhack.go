package main

import (
	"encoding/xml"
	"flag"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
)

const (
	imgsrc    = "<img src=\""
	imgsrcLen = len(imgsrc)
)

type Item struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
}

type Feed struct {
	Title       string  `xml:"channel>title"`
	Description string  `xml:"channel>description"`
	Item        []*Item `xml:"channel>item"`
}

type BySize []*gif.GIF

func (g BySize) Len() int      { return len(g) }
func (g BySize) Swap(i, j int) { g[i], g[j] = g[j], g[i] }
func (g BySize) Less(i, j int) bool {
	isize := g[i].Image[0].Rect.Size()
	jsize := g[j].Image[0].Rect.Size()
	return (isize.X * isize.Y) > (jsize.X * jsize.Y)
}

func main() {
	rssURL := flag.String("rss", "", "rss url - ie http://61cygni.tumblr.com/rss or http://derekg.org/rss")
	outname := flag.String("out", "out.gif", "filename to dump the animate GIF to")
	flag.Parse()
	if *rssURL == "" {
		log.Fatal("Yo dumb ass put in a blog url")
	}
	resp, err := http.Get(*rssURL)
	if err != nil {
		log.Fatal(err)
	}
	// grab the rss feed
	rss := Feed{}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	err = xml.Unmarshal(body, &rss)
	if err != nil {
		log.Fatal(err)
	}

	// rip out the image urls
	imgUrls := []string{}
	for _, it := range rss.Item {
		i := strings.Index(it.Description, imgsrc)
		if i != -1 {
			i += imgsrcLen
			imgUrls = append(imgUrls, it.Description[i:i+strings.Index(it.Description[i:], "\"")])
		}
	}

	var wg sync.WaitGroup
	results := make(chan *gif.GIF, len(imgUrls))
	// fetch the images
	for _, iurl := range imgUrls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			resp, err := http.Get(u)
			if err != nil {
				log.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.Header.Get("Content-Type") == "image/gif" {
				img, err := gif.DecodeAll(resp.Body)
				if err != nil {
					log.Fatal(err)
				}
				results <- img
			} else {
				img, _, err := image.Decode(resp.Body)
				if err != nil {
					log.Fatal(err)
				}
				pr, pw := io.Pipe()
				// convert all to a GIF
				go func() {
					err = gif.Encode(pw, img, &gif.Options{NumColors: 32})
					if err != nil {
						log.Fatal(err)
					}
				}()
				g, err := gif.DecodeAll(pr)
				if err != nil {
					log.Fatal(err)
				}
				results <- g
			}
		}(iurl)
	}
	wg.Wait()
	close(results)

	all := make([]*gif.GIF, 0)
	for g := range results {
		all = append(all, g)
	}
	sort.Sort(BySize(all)) // get the biggest at the front of the list

	montage := make([]*image.Paletted, 0)
	delay := make([]int, 0)
	for _, g := range all {
		montage = append(montage, g.Image...)
		if len(g.Delay) == 1 {
			g.Delay[0] = 40
		}
		delay = append(delay, g.Delay...)
	}
	f, err := os.Create(*outname)
	defer f.Close()
	err = gif.EncodeAll(f, &gif.GIF{Image: montage, Delay: delay, LoopCount: -1})
}
