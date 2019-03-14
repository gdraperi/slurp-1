package external

import (
    "fmt"
    "os"
    "io"
    "io/ioutil"
    "encoding/json"
    "strings"
    "time"
    "net/http"

    "slurp/scanner/cmd"

    "github.com/joeguo/tldextract"
    "golang.org/x/net/idna"
    "github.com/jmoiron/jsonq"
    "github.com/Workiva/go-datastructures/queue"

    log "github.com/sirupsen/logrus"
)

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

var domainQ *queue.Queue
var permutatedQ *queue.Queue
var extract *tldextract.TLDExtract
var sem chan int
var kclient *http.Client

// PermutateDomain returns all possible domain permutations
func PermutateDomain(domain, suffix, cfgPermutationsFile string) []string {
	if _, err := os.Stat(cfgPermutationsFile); err != nil {
		log.Fatal(err)
	}

	jsondata, err := ioutil.ReadFile(cfgPermutationsFile)

	if err != nil {
		log.Fatal(err)
	}

	data := map[string]interface{}{}
	dec := json.NewDecoder(strings.NewReader(string(jsondata)))
	dec.Decode(&data)
	jq := jsonq.NewQuery(data)

	s3url, err := jq.String("s3_url")

	if err != nil {
		log.Fatal(err)
	}

	var permutations []string

	perms, err := jq.Array("permutations")

	if err != nil {
		log.Fatal(err)
	}

	// Our list of permutations
	for i := range perms {
		permutations = append(permutations, fmt.Sprintf(perms[i].(string), domain, s3url))
	}

	// Permutations that are not easily put into the list
	permutations = append(permutations, fmt.Sprintf("%s.%s.%s", domain, suffix, s3url))
	permutations = append(permutations, fmt.Sprintf("%s.%s", strings.Replace(fmt.Sprintf("%s.%s", domain, suffix), ".", "", -1), s3url))

	return permutations
}

// PermutateKeyword returns all possible keyword permutations
func PermutateKeyword(keyword, cfgPermutationsFile string) []string {
	if _, err := os.Stat(cfgPermutationsFile); err != nil {
		log.Fatal(err)
	}

	jsondata, err := ioutil.ReadFile(cfgPermutationsFile)

	if err != nil {
		log.Fatal(err)
	}

	data := map[string]interface{}{}
	dec := json.NewDecoder(strings.NewReader(string(jsondata)))
	dec.Decode(&data)
	jq := jsonq.NewQuery(data)

	s3url, err := jq.String("s3_url")

	if err != nil {
		log.Fatal(err)
	}

	var permutations []string

	perms, err := jq.Array("permutations")

	if err != nil {
		log.Fatal(err)
	}

	// Our list of permutations
	for i := range perms {
		permutations = append(permutations, fmt.Sprintf(perms[i].(string), keyword, s3url))
	}

	return permutations
}

// PermutateDomainRunner stores the dbQ results into the database
func PermutateDomainRunner(cfg *cmd.Config) {
	for i := range cfg.Domains {
		if len(cfg.Domains[i]) != 0 {
			punyCfgDomain, err := idna.ToASCII(cfg.Domains[i])
			if err != nil {
				log.Fatal(err)
			}

			if cfg.Domains[i] != punyCfgDomain {
				log.Infof("Domain %s is %s (punycode)", cfg.Domains[i], punyCfgDomain)
				log.Errorf("Internationalized domains cannot be S3 buckets (%s)", cfg.Domains[i])
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
				Raw:    cfg.Domains[i],
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

		pd := PermutateDomain(d.Domain, d.Suffix, cfg.PermutationsFile)

		for p := range pd {
			permutatedQ.Put(PermutatedDomain{
				Permutation: pd[p],
				Domain:      d,
			})
		}
	}
}

// PermutateKeywordRunner stores the dbQ results into the database
func PermutateKeywordRunner(cfg *cmd.Config) {
	for keyword := range cfg.Keywords {
		pd := PermutateKeyword(cfg.Keywords[keyword], cfg.PermutationsFile)

		for p := range pd {
			permutatedQ.Put(Keyword{
				Keyword:     cfg.Keywords[keyword],
				Permutation: pd[p],
			})
		}
	}
}

// CheckDomainPermutations runs through all permutations checking them for PUBLIC/FORBIDDEN buckets
func CheckDomainPermutations(cfg *cmd.Config) {
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
				cfg.Stats.IncRequests200()
				cfg.Stats.Add200Link(pd.Permutation)
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
					cfg.Stats.IncRequests200()
					cfg.Stats.Add200Link(loc)
				} else if resp.StatusCode == 403 {
					log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix)
					cfg.Stats.IncRequests403()
					cfg.Stats.Add403Link(pd.Permutation)
				}
			} else if resp.StatusCode == 403 {
				log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix)
				cfg.Stats.IncRequests403()
				cfg.Stats.Add403Link(pd.Permutation)
			} else if resp.StatusCode == 404 {
				log.Debugf("\033[31m\033[1mNOT FOUND\033[39m\033[0m http://%s (\033[33mhttp://%s.%s\033[39m)", pd.Permutation, pd.Domain.Domain, pd.Domain.Suffix)
				cfg.Stats.IncRequests404()
				cfg.Stats.Add404Link(pd.Permutation)
			} else if resp.StatusCode == 503 {
				log.Infof("\033[31m\033[1mTOO FAST\033[39m\033[0m (added to queue to process later)")
				permutatedQ.Put(pd)
				cfg.Stats.IncRequests503()
				cfg.Stats.Add503Link(pd.Permutation)
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
func CheckKeywordPermutations(cfg *cmd.Config) {
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
				cfg.Stats.IncRequests200()
				cfg.Stats.Add200Link(pd.Permutation)
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
					cfg.Stats.IncRequests200()
					cfg.Stats.Add200Link(loc)
				} else if resp.StatusCode == 403 {
					log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m %s (\033[33m%s\033[39m)", loc, pd.Keyword)
					cfg.Stats.IncRequests403()
					cfg.Stats.Add403Link(loc)
				}
			} else if resp.StatusCode == 403 {
				log.Infof("\033[31m\033[1mFORBIDDEN\033[39m\033[0m http://%s (\033[33m%s\033[39m)", pd.Permutation, pd.Keyword)
				cfg.Stats.IncRequests403()
				cfg.Stats.Add403Link(pd.Permutation)
			} else if resp.StatusCode == 404 {
				log.Debugf("\033[31m\033[1mNOT FOUND\033[39m\033[0m http://%s (\033[33m%s\033[39m)", pd.Permutation, pd.Keyword)
				cfg.Stats.IncRequests404()
				cfg.Stats.Add404Link(pd.Permutation)
			} else if resp.StatusCode == 503 {
				log.Infof("\033[31m\033[1mTOO FAST\033[39m\033[0m (added to queue to process later)")
				permutatedQ.Put(pd)
				cfg.Stats.IncRequests503()
				cfg.Stats.Add503Link(pd.Permutation)
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

// Init does low level initialization before we can run
func Init(cfg *cmd.Config) {
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
