// slurp s3 bucket enumerator
// Copyright (C) 2017 8c30ff1057d69a6a6f6dc2212d8ec25196c542acb8620eb4148318a4b10dd131
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
//

package main

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/joeguo/tldextract"
	"golang.org/x/net/idna"

	"github.com/Workiva/go-datastructures/queue"
	log "github.com/sirupsen/logrus"

    "slurp/scanner/external"
    "slurp/scanner/stats"
    "slurp/scanner/cmd"
)

var kclient *http.Client
var domainQ *queue.Queue
var permutatedQ *queue.Queue
var extract *tldextract.TLDExtract
var sem chan int
var action string

// Global config
var st *stats.Stats
var cfg *cmd.Config

// Domain is used when `domain` action is used
type Domain struct {
	CN     string
	Domain string
	Suffix string
	Raw    string
}

// Keyword is used when `keyword` action is used
type Keyword struct {
	Permutation string
	Keyword     string
}

// PermutatedDomain is a permutation of the domain
type PermutatedDomain struct {
	Permutation string
	Domain      Domain
}

// Init does low level initialization before we can run
func Init() {
	var err error

	domainQ = queue.New(1000)
	permutatedQ = queue.New(1000)

	extract, err = tldextract.New("./tld.cache", false)

	if err != nil {
		log.Fatal(err)
	}

	tr := &http.Transport{
		IdleConnTimeout:       1 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		MaxIdleConnsPerHost:   cfg.Concurrency * 4,
		MaxIdleConns:          cfg.Concurrency,
		ExpectContinueTimeout: 1 * time.Second,
	}

	kclient = &http.Client{
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// PermutateDomainRunner stores the dbQ results into the database
func PermutateDomainRunner(domains []string) {
	for i := range domains {
		if len(domains[i]) != 0 {
			punyCfgDomain, err := idna.ToASCII(domains[i])
			if err != nil {
				log.Fatal(err)
			}

			if domains[i] != punyCfgDomain {
				log.Infof("Domain %s is %s (punycode)", domains[i], punyCfgDomain)
				log.Errorf("Internationalized domains cannot be S3 buckets (%s)", domains[i])
				continue
			}

			result := extract.Extract(punyCfgDomain)

			if result.Root == "" || result.Tld == "" {
				log.Errorf("%s is not a valid domain", punyCfgDomain)
				continue
			}

			domainQ.Put(Domain{
				CN:     punyCfgDomain,
				Domain: result.Root,
				Suffix: result.Tld,
				Raw:    domains[i],
			})
		}
	}

	if domainQ.Len() == 0 {
		os.Exit(1)
	}

	for {
		dstruct, err := domainQ.Get(1)

		if err != nil {
			log.Error(err)
			continue
		}

		var d Domain = dstruct[0].(Domain)

		//log.Infof("CN: %s\tDomain: %s.%s", d.CN, d.Domain, d.Suffix)

		pd := external.PermutateDomain(d.Domain, d.Suffix, cfg.PermutationsFile)

		for p := range pd {
			permutatedQ.Put(PermutatedDomain{
				Permutation: pd[p],
				Domain:      d,
			})
		}
	}
}

// PermutateKeywordRunner stores the dbQ results into the database
func PermutateKeywordRunner(keywords []string) {
	for keyword := range keywords {
		pd := external.PermutateKeyword(keywords[keyword], cfg.PermutationsFile)

		for p := range pd {
			permutatedQ.Put(Keyword{
				Keyword:     keywords[keyword],
				Permutation: pd[p],
			})
		}
	}
}

// CheckDomainPermutations runs through all permutations checking them for PUBLIC/FORBIDDEN buckets
func CheckDomainPermutations() {
	var max = cfg.Concurrency
	sem = make(chan int, max)

	for {
		sem <- 1
		dom, err := permutatedQ.Get(1)

		if err != nil {
			log.Error(err)
		}

		go func(pd PermutatedDomain) {
			time.Sleep(500 * time.Millisecond)
			req, err := http.NewRequest("GET", "http://s3-1-w.amazonaws.com", nil)

			if err != nil {
				if !strings.Contains(err.Error(), "time") {
					log.Error(err)
				}

				permutatedQ.Put(pd)
				<-sem
				return
			}

			req.Host = pd.Permutation

			resp, err1 := kclient.Do(req)

			if err1 != nil {
				if strings.Contains(err1.Error(), "time") {
					permutatedQ.Put(pd)
					<-sem
					return
				}

				log.Error(err1)
				permutatedQ.Put(pd)
				<-sem
				return
			}
			io.Copy(ioutil.Discard, resp.Body)
			defer resp.Body.Close()

			if resp.StatusCode == 200 {
				log.Infof("\033[32m\033[1mPUBLIC\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix)
				st.IncRequests200()
				st.Add200Link(pd.Permutation)
			} else if resp.StatusCode == 307 {
				loc := resp.Header.Get("Location")

				req, err := http.NewRequest("GET", loc, nil)

				if err != nil {
					log.Error(err)
				}

				resp, err1 := kclient.Do(req)

				if err1 != nil {
					if strings.Contains(err1.Error(), "time") {
						permutatedQ.Put(pd)
						<-sem
						return
					}

					log.Error(err1)
					permutatedQ.Put(pd)
					<-sem
					return
				}

				defer resp.Body.Close()

				if resp.StatusCode == 200 {
					log.Infof("\033[32m\033[1mPUBLIC\033[39m\033[0m %s (\033[33mhttp://%s.%s\033[39m)", loc, pd.Domain.Domain, pd.Domain.Suffix)
					st.IncRequests200()
					st.Add200Link(loc)
				} else if resp.StatusCode == 403 {
					log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix)
					st.IncRequests403()
					st.Add403Link(pd.Permutation)
				}
			} else if resp.StatusCode == 403 {
				log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix)
				st.IncRequests403()
				st.Add403Link(pd.Permutation)
			} else if resp.StatusCode == 404 {
				log.Debugf("\033[31m\033[1mNOT FOUND\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix)
				st.IncRequests404()
				st.Add404Link(pd.Permutation)
			} else if resp.StatusCode == 503 {
				log.Infof("\033[31m\033[1mTOO FAST\033[39m\033[0m (added to queue to process later)")
				permutatedQ.Put(pd)
				st.IncRequests503()
				st.Add503Link(pd.Permutation)
			} else {
				log.Infof("\033[34m\033[1mUNKNOWN\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m) (%d)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix, resp.StatusCode)
			}

			<-sem
		}(dom[0].(PermutatedDomain))

		if permutatedQ.Len() == 0 {
			break
		}
	}
}

// CheckKeywordPermutations runs through all permutations checking them for PUBLIC/FORBIDDEN buckets
func CheckKeywordPermutations() {
	var max = cfg.Concurrency
	sem = make(chan int, max)

	for {
		sem <- 1
		dom, err := permutatedQ.Get(1)

		if err != nil {
			log.Error(err)
		}

		go func(pd Keyword) {
			time.Sleep(500 * time.Millisecond)
			req, err := http.NewRequest("GET", "http://s3-1-w.amazonaws.com", nil)

			if err != nil {
				if !strings.Contains(err.Error(), "time") {
					log.Error(err)
				}

				permutatedQ.Put(pd)
				<-sem
				return
			}

			req.Host = pd.Permutation
			//req.Header.Add("Host", host)

			resp, err1 := kclient.Do(req)

			if err1 != nil {
				if strings.Contains(err1.Error(), "time") {
					permutatedQ.Put(pd)
					<-sem
					return
				}

				log.Error(err1)
				permutatedQ.Put(pd)
				<-sem
				return
			}
			io.Copy(ioutil.Discard, resp.Body)
			defer resp.Body.Close()

			//log.Infof("%s (%d)", host, resp.StatusCode)
			if resp.StatusCode == 200 {
				log.Infof("\033[32m\033[1mPUBLIC\033[39m\033[0m http://%s (\033[33m%s\033[39m)", pd.Permutation, pd.Keyword)
				st.IncRequests200()
				st.Add200Link(pd.Permutation)
			} else if resp.StatusCode == 307 {
				loc := resp.Header.Get("Location")

				req, err := http.NewRequest("GET", loc, nil)

				if err != nil {
					log.Error(err)
				}

				resp, err1 := kclient.Do(req)

				if err1 != nil {
					if strings.Contains(err1.Error(), "time") {
						permutatedQ.Put(pd)
						<-sem
						return
					}

					log.Error(err1)
					permutatedQ.Put(pd)
					<-sem
					return
				}

				defer resp.Body.Close()

				if resp.StatusCode == 200 {
					log.Infof("\033[32m\033[1mPUBLIC\033[39m\033[0m %s (\033[33m%s\033[39m)", loc, pd.Keyword)
					st.IncRequests200()
					st.Add200Link(loc)
				} else if resp.StatusCode == 403 {
					log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m %s (\033[33m%s\033[39m)", loc, pd.Keyword)
					st.IncRequests403()
					st.Add403Link(loc)
				}
			} else if resp.StatusCode == 403 {
				log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m http://%s (\033[33m%s\033[39m)", pd.Permutation, pd.Keyword)
				st.IncRequests403()
				st.Add403Link(pd.Permutation)
			} else if resp.StatusCode == 404 {
				log.Debugf("\033[31m\033[1mNOT FOUND\033[39m\033[0m http://%s (\033[33m%s\033[39m)", pd.Permutation, pd.Keyword)
				st.IncRequests404()
				st.Add404Link(pd.Permutation)
			} else if resp.StatusCode == 503 {
				log.Infof("\033[31m\033[1mTOO FAST\033[39m\033[0m (added to queue to process later)")
				permutatedQ.Put(pd)
				st.IncRequests503()
				st.Add503Link(pd.Permutation)
			} else {
				log.Infof("\033[34m\033[1mUNKNOWN\033[39m\033[0m http://%s (\033[33m%s\033[39m) (%d)", pd.Permutation, pd.Keyword, resp.StatusCode)
			}

			<-sem
		}(dom[0].(Keyword))

		if permutatedQ.Len() == 0 {
			break
		}
	}
}

func main() {
    c := cmd.CmdInit("slurp", "Public buckets finder", "Public buckets finder")
    cfg = &c
    st = c.Stats

	switch c.State {
	case "DOMAIN":
		Init()

		log.Info("Building permutations....")
		PermutateDomainRunner(c.Domains)

		log.Info("Processing permutations....")
		CheckDomainPermutations()

	case "KEYWORD":
		Init()

		log.Info("Building permutations....")
		PermutateKeywordRunner(c.Keywords)

		log.Info("Processing permutations....")
		CheckKeywordPermutations()

	case "NADA":
		log.Info("Check help")
		os.Exit(0)
	}

	// Print stats info
	log.Printf("%+v", st)
}
