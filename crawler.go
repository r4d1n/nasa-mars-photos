package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
)

type Scraper struct {
	APIKey    string
	AWSRegion string
	S3Bucket  string
}

func (s Scraper) crawl(name string) error {
	last, err := checkLastInsert(name)
	if err != nil {
		return err
	}
	murl := fmt.Sprint("https://api.nasa.gov/mars-photos/api/v1/manifests/", name, "?api_key=", s.APIKey)
	res, err := http.Get(murl)
	if err != nil {
		return err
	} else {
		decoder := json.NewDecoder(res.Body)
		defer res.Body.Close()
		var r manifestResponse
		err := decoder.Decode(&r)
		sort.Sort(r.Manifest.Sols)
		if err != nil {
			return err
		}
		// make a Sol for most recent photo Sol and get index in manifest sols to find initial loop position
		d := Sol{Sol: last.Sol}
		i := r.Manifest.Sols.IndexOf(d)
		// need to advance if nothing has been saved or if all photos have been found
		count, err := checkTotalSaved(name, d.Sol)
		if err != nil {
			return err
		}
		if i == -1 || r.Manifest.Sols[i].TotalPhotos == count {
			i++
		}
		for ; i < len(r.Manifest.Sols); i++ {
			purl := fmt.Sprint("https://api.nasa.gov/mars-photos/api/v1/rovers/", name, "/photos?sol=", r.Manifest.Sols[i].Sol, "&api_key=", s.APIKey)
			photos, err := getPhotos(purl)
			if err != nil {
				return err
			}
			index := photos.IndexOf(last)
			// make start looping from initial position determined by last photo saved
			for j := index + 1; j < len(photos); j++ {
				ph := photos[j]
				ph.Rover = name
				err := ph.copyToS3(s.AWSRegion, s.S3Bucket)
				if err != nil {
					return err
				}
				err = ph.save()
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// find the last saved photo from this rover
func checkLastInsert(rover string) (Photo, error) {
	var p Photo
	err := db.QueryRow("select id, sol from photos where rover=$1 order by id desc limit 1", rover).Scan(&p.Id, &p.Sol)
	if err == sql.ErrNoRows {
		return p, nil
	} else if err != nil {
		return p, fmt.Errorf("retrieving last photo from rover %s: %v", rover, err)
	}
	return p, nil
}

func checkTotalSaved(rover string, sol int) (int, error) {
	var count int
	err := db.QueryRow("select count(*) from photos where rover=$1", rover).Scan(&count)
	if err == sql.ErrNoRows {
		return count, nil
	} else if err != nil {
		return count, fmt.Errorf("retrieving count from rover %s for sol %d: %v", rover, sol, err)
	}
	return count, nil
}

type photoResponse struct {
	Photos Photos
}

func getPhotos(url string) (Photos, error) {
	var pr photoResponse
	res, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("retrieving url %s: %v", url, err)
	} else {
		defer res.Body.Close()
		decoder := json.NewDecoder(res.Body)
		err := decoder.Decode(&pr)
		if err != nil {
			return nil, fmt.Errorf("parsing photo json: %v", err)
		}
	}
	sort.Sort(pr.Photos)
	return pr.Photos, nil
}
